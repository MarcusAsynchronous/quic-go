package quic

import (
	"errors"

	"github.com/lucas-clemente/quic-go/internal/handshake"

	"github.com/lucas-clemente/quic-go/internal/protocol"
	"github.com/lucas-clemente/quic-go/qerr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Streams Map", func() {
	var (
		m *streamsMap
	)

	setNewStreamsMap := func(p protocol.Perspective) {
		newStream := func(id protocol.StreamID) *stream {
			return newStream(id, func() {}, nil, nil)
		}
		removeStreamCallback := func(protocol.StreamID) {}
		m = newStreamsMap(newStream, removeStreamCallback, p)
	}

	AfterEach(func() {
		Expect(m.openStreams).To(HaveLen(len(m.streams)))
	})

	deleteStream := func(id protocol.StreamID) {
		str := m.streams[id]
		Expect(str).ToNot(BeNil())
		str.cancelled.Set(true)
		Expect(str.finished()).To(BeTrue())
		err := m.DeleteClosedStreams()
		Expect(err).ToNot(HaveOccurred())
	}

	Context("getting and creating streams", func() {
		Context("as a server", func() {
			BeforeEach(func() {
				setNewStreamsMap(protocol.PerspectiveServer)
			})

			Context("client-side streams", func() {
				It("gets new streams", func() {
					s, err := m.GetOrOpenStream(1)
					Expect(err).NotTo(HaveOccurred())
					Expect(s.StreamID()).To(Equal(protocol.StreamID(1)))
					Expect(m.numIncomingStreams).To(BeEquivalentTo(1))
					Expect(m.numOutgoingStreams).To(BeZero())
				})

				It("rejects streams with even IDs", func() {
					_, err := m.GetOrOpenStream(6)
					Expect(err).To(MatchError("InvalidStreamID: attempted to open stream 6 from client-side"))
				})

				It("rejects streams with even IDs, which are lower thatn the highest client-side stream", func() {
					_, err := m.GetOrOpenStream(5)
					Expect(err).NotTo(HaveOccurred())
					_, err = m.GetOrOpenStream(4)
					Expect(err).To(MatchError("InvalidStreamID: attempted to open stream 4 from client-side"))
				})

				It("gets existing streams", func() {
					s, err := m.GetOrOpenStream(5)
					Expect(err).NotTo(HaveOccurred())
					numStreams := m.numIncomingStreams
					s, err = m.GetOrOpenStream(5)
					Expect(err).NotTo(HaveOccurred())
					Expect(s.StreamID()).To(Equal(protocol.StreamID(5)))
					Expect(m.numIncomingStreams).To(Equal(numStreams))
				})

				It("returns nil for closed streams", func() {
					_, err := m.GetOrOpenStream(5)
					Expect(err).NotTo(HaveOccurred())
					deleteStream(5)
					s, err := m.GetOrOpenStream(5)
					Expect(err).NotTo(HaveOccurred())
					Expect(s).To(BeNil())
				})

				It("opens skipped streams", func() {
					_, err := m.GetOrOpenStream(5)
					Expect(err).NotTo(HaveOccurred())
					Expect(m.streams).To(HaveKey(protocol.StreamID(1)))
					Expect(m.streams).To(HaveKey(protocol.StreamID(3)))
					Expect(m.streams).To(HaveKey(protocol.StreamID(5)))
				})

				It("doesn't reopen an already closed stream", func() {
					_, err := m.GetOrOpenStream(5)
					Expect(err).ToNot(HaveOccurred())
					deleteStream(5)
					Expect(err).ToNot(HaveOccurred())
					str, err := m.GetOrOpenStream(5)
					Expect(err).ToNot(HaveOccurred())
					Expect(str).To(BeNil())
				})

				Context("counting streams", func() {
					It("errors when too many streams are opened", func() {
						for i := uint32(0); i < m.maxIncomingStreams; i++ {
							_, err := m.GetOrOpenStream(protocol.StreamID(i*2 + 1))
							Expect(err).NotTo(HaveOccurred())
						}
						_, err := m.GetOrOpenStream(protocol.StreamID(2*m.maxIncomingStreams + 3))
						Expect(err).To(MatchError(qerr.TooManyOpenStreams))
					})

					It("errors when too many streams are opened implicitely", func() {
						_, err := m.GetOrOpenStream(protocol.StreamID(m.maxIncomingStreams*2 + 1))
						Expect(err).To(MatchError(qerr.TooManyOpenStreams))
					})

					It("does not error when many streams are opened and closed", func() {
						for i := uint32(2); i < 10*m.maxIncomingStreams; i++ {
							str, err := m.GetOrOpenStream(protocol.StreamID(i*2 + 1))
							Expect(err).NotTo(HaveOccurred())
							deleteStream(str.StreamID())
						}
					})
				})
			})

			Context("server-side streams", func() {
				It("doesn't allow opening streams before receiving the transport parameters", func() {
					_, err := m.OpenStream()
					Expect(err).To(MatchError(qerr.TooManyOpenStreams))
				})

				It("opens a stream 2 first", func() {
					m.UpdateTransportParameters(&handshake.TransportParameters{MaxStreams: 100})
					s, err := m.OpenStream()
					Expect(err).ToNot(HaveOccurred())
					Expect(s).ToNot(BeNil())
					Expect(s.StreamID()).To(Equal(protocol.StreamID(2)))
					Expect(m.numIncomingStreams).To(BeZero())
					Expect(m.numOutgoingStreams).To(BeEquivalentTo(1))
				})

				It("returns the error when the streamsMap was closed", func() {
					testErr := errors.New("test error")
					m.CloseWithError(testErr)
					_, err := m.OpenStream()
					Expect(err).To(MatchError(testErr))
				})

				It("doesn't reopen an already closed stream", func() {
					m.UpdateTransportParameters(&handshake.TransportParameters{MaxStreams: 100})
					str, err := m.OpenStream()
					Expect(err).ToNot(HaveOccurred())
					Expect(str.StreamID()).To(Equal(protocol.StreamID(2)))
					deleteStream(2)
					Expect(err).ToNot(HaveOccurred())
					str, err = m.GetOrOpenStream(2)
					Expect(err).ToNot(HaveOccurred())
					Expect(str).To(BeNil())
				})

				Context("counting streams", func() {
					const maxOutgoingStreams = 50

					BeforeEach(func() {
						m.UpdateTransportParameters(&handshake.TransportParameters{MaxStreams: maxOutgoingStreams})
					})

					It("errors when too many streams are opened", func() {
						for i := 1; i <= maxOutgoingStreams; i++ {
							_, err := m.OpenStream()
							Expect(err).NotTo(HaveOccurred())
						}
						_, err := m.OpenStream()
						Expect(err).To(MatchError(qerr.TooManyOpenStreams))
					})

					It("does not error when many streams are opened and closed", func() {
						for i := 2; i < 10*maxOutgoingStreams; i++ {
							str, err := m.OpenStream()
							Expect(err).NotTo(HaveOccurred())
							deleteStream(str.StreamID())
						}
					})

					It("allows many server- and client-side streams at the same time", func() {
						for i := 1; i < maxOutgoingStreams; i++ {
							_, err := m.OpenStream()
							Expect(err).ToNot(HaveOccurred())
						}
						for i := 0; i < maxOutgoingStreams; i++ {
							_, err := m.GetOrOpenStream(protocol.StreamID(2*i + 1))
							Expect(err).ToNot(HaveOccurred())
						}
					})
				})

				Context("opening streams synchronously", func() {
					const maxOutgoingStreams = 10

					BeforeEach(func() {
						m.UpdateTransportParameters(&handshake.TransportParameters{MaxStreams: maxOutgoingStreams})
					})

					openMaxNumStreams := func() {
						for i := 1; i <= maxOutgoingStreams; i++ {
							_, err := m.OpenStream()
							Expect(err).NotTo(HaveOccurred())
						}
						_, err := m.OpenStream()
						Expect(err).To(MatchError(qerr.TooManyOpenStreams))
					}

					It("waits until another stream is closed", func() {
						openMaxNumStreams()
						var returned bool
						var str *stream
						go func() {
							defer GinkgoRecover()
							var err error
							str, err = m.OpenStreamSync()
							Expect(err).ToNot(HaveOccurred())
							returned = true
						}()

						Consistently(func() bool { return returned }).Should(BeFalse())
						deleteStream(6)
						Eventually(func() bool { return returned }).Should(BeTrue())
						Expect(str.StreamID()).To(Equal(protocol.StreamID(2*maxOutgoingStreams + 2)))
					})

					It("stops waiting when an error is registered", func() {
						openMaxNumStreams()
						testErr := errors.New("test error")
						var err error
						var returned bool
						go func() {
							_, err = m.OpenStreamSync()
							returned = true
						}()

						Consistently(func() bool { return returned }).Should(BeFalse())
						m.CloseWithError(testErr)
						Eventually(func() bool { return returned }).Should(BeTrue())
						Expect(err).To(MatchError(testErr))
					})

					It("immediately returns when OpenStreamSync is called after an error was registered", func() {
						testErr := errors.New("test error")
						m.CloseWithError(testErr)
						_, err := m.OpenStreamSync()
						Expect(err).To(MatchError(testErr))
					})
				})
			})

			Context("accepting streams", func() {
				It("does nothing if no stream is opened", func() {
					var accepted bool
					go func() {
						_, _ = m.AcceptStream()
						accepted = true
					}()
					Consistently(func() bool { return accepted }).Should(BeFalse())
				})

				It("accepts stream 1 first", func() {
					var str *stream
					go func() {
						defer GinkgoRecover()
						var err error
						str, err = m.AcceptStream()
						Expect(err).ToNot(HaveOccurred())
					}()
					_, err := m.GetOrOpenStream(1)
					Expect(err).ToNot(HaveOccurred())
					Eventually(func() Stream { return str }).ShouldNot(BeNil())
					Expect(str.StreamID()).To(Equal(protocol.StreamID(1)))
				})

				It("returns an implicitly opened stream, if a stream number is skipped", func() {
					var str *stream
					go func() {
						defer GinkgoRecover()
						var err error
						str, err = m.AcceptStream()
						Expect(err).ToNot(HaveOccurred())
					}()
					_, err := m.GetOrOpenStream(5)
					Expect(err).ToNot(HaveOccurred())
					Eventually(func() Stream { return str }).ShouldNot(BeNil())
					Expect(str.StreamID()).To(Equal(protocol.StreamID(1)))
				})

				It("returns to multiple accepts", func() {
					var str1, str2 *stream
					go func() {
						defer GinkgoRecover()
						var err error
						str1, err = m.AcceptStream()
						Expect(err).ToNot(HaveOccurred())
					}()
					go func() {
						defer GinkgoRecover()
						var err error
						str2, err = m.AcceptStream()
						Expect(err).ToNot(HaveOccurred())
					}()
					_, err := m.GetOrOpenStream(3) // opens stream 1 and 3
					Expect(err).ToNot(HaveOccurred())
					Eventually(func() *stream { return str1 }).ShouldNot(BeNil())
					Eventually(func() *stream { return str2 }).ShouldNot(BeNil())
					Expect(str1.StreamID()).ToNot(Equal(str2.StreamID()))
					Expect(str1.StreamID() + str2.StreamID()).To(BeEquivalentTo(1 + 3))
				})

				It("waits a new stream is available", func() {
					var str *stream
					go func() {
						defer GinkgoRecover()
						var err error
						str, err = m.AcceptStream()
						Expect(err).ToNot(HaveOccurred())
					}()
					Consistently(func() *stream { return str }).Should(BeNil())
					_, err := m.GetOrOpenStream(1)
					Expect(err).ToNot(HaveOccurred())
					Eventually(func() *stream { return str }).ShouldNot(BeNil())
					Expect(str.StreamID()).To(Equal(protocol.StreamID(1)))
				})

				It("returns multiple streams on subsequent Accept calls, if available", func() {
					var str *stream
					go func() {
						defer GinkgoRecover()
						var err error
						str, err = m.AcceptStream()
						Expect(err).ToNot(HaveOccurred())
					}()
					_, err := m.GetOrOpenStream(3)
					Expect(err).ToNot(HaveOccurred())
					Eventually(func() *stream { return str }).ShouldNot(BeNil())
					Expect(str.StreamID()).To(Equal(protocol.StreamID(1)))
					str, err = m.AcceptStream()
					Expect(err).ToNot(HaveOccurred())
					Expect(str.StreamID()).To(Equal(protocol.StreamID(3)))
				})

				It("blocks after accepting a stream", func() {
					var accepted bool
					_, err := m.GetOrOpenStream(1)
					Expect(err).ToNot(HaveOccurred())
					str, err := m.AcceptStream()
					Expect(err).ToNot(HaveOccurred())
					Expect(str.StreamID()).To(Equal(protocol.StreamID(1)))
					go func() {
						defer GinkgoRecover()
						_, _ = m.AcceptStream()
						accepted = true
					}()
					Consistently(func() bool { return accepted }).Should(BeFalse())
				})

				It("stops waiting when an error is registered", func() {
					testErr := errors.New("testErr")
					var acceptErr error
					go func() {
						_, acceptErr = m.AcceptStream()
					}()
					Consistently(func() error { return acceptErr }).ShouldNot(HaveOccurred())
					m.CloseWithError(testErr)
					Eventually(func() error { return acceptErr }).Should(MatchError(testErr))
				})

				It("immediately returns when Accept is called after an error was registered", func() {
					testErr := errors.New("testErr")
					m.CloseWithError(testErr)
					_, err := m.AcceptStream()
					Expect(err).To(MatchError(testErr))
				})
			})
		})

		Context("as a client", func() {
			BeforeEach(func() {
				setNewStreamsMap(protocol.PerspectiveClient)
			})

			Context("client-side streams", func() {
				It("rejects streams with odd IDs", func() {
					_, err := m.GetOrOpenStream(5)
					Expect(err).To(MatchError("InvalidStreamID: attempted to open stream 5 from server-side"))
				})

				It("rejects streams with odds IDs, which are lower thatn the highest server-side stream", func() {
					_, err := m.GetOrOpenStream(6)
					Expect(err).NotTo(HaveOccurred())
					_, err = m.GetOrOpenStream(5)
					Expect(err).To(MatchError("InvalidStreamID: attempted to open stream 5 from server-side"))
				})

				It("gets new streams", func() {
					s, err := m.GetOrOpenStream(2)
					Expect(err).NotTo(HaveOccurred())
					Expect(s.StreamID()).To(Equal(protocol.StreamID(2)))
					Expect(m.numOutgoingStreams).To(BeEquivalentTo(1))
					Expect(m.numIncomingStreams).To(BeZero())
				})

				It("opens skipped streams", func() {
					_, err := m.GetOrOpenStream(6)
					Expect(err).NotTo(HaveOccurred())
					Expect(m.streams).To(HaveKey(protocol.StreamID(2)))
					Expect(m.streams).To(HaveKey(protocol.StreamID(4)))
					Expect(m.streams).To(HaveKey(protocol.StreamID(6)))
				})

				It("doesn't reopen an already closed stream", func() {
					str, err := m.OpenStream()
					Expect(err).ToNot(HaveOccurred())
					Expect(str.StreamID()).To(Equal(protocol.StreamID(1)))
					deleteStream(1)
					Expect(err).ToNot(HaveOccurred())
					str, err = m.GetOrOpenStream(1)
					Expect(err).ToNot(HaveOccurred())
					Expect(str).To(BeNil())
				})
			})

			Context("server-side streams", func() {
				It("opens stream 1 first", func() {
					s, err := m.OpenStream()
					Expect(err).ToNot(HaveOccurred())
					Expect(s).ToNot(BeNil())
					Expect(s.StreamID()).To(BeEquivalentTo(1))
					Expect(m.numOutgoingStreams).To(BeZero())
					Expect(m.numIncomingStreams).To(BeEquivalentTo(1))
				})

				It("opens multiple streams", func() {
					s1, err := m.OpenStream()
					Expect(err).ToNot(HaveOccurred())
					s2, err := m.OpenStream()
					Expect(err).ToNot(HaveOccurred())
					Expect(s2.StreamID()).To(Equal(s1.StreamID() + 2))
				})

				It("doesn't reopen an already closed stream", func() {
					_, err := m.GetOrOpenStream(4)
					Expect(err).ToNot(HaveOccurred())
					deleteStream(4)
					Expect(err).ToNot(HaveOccurred())
					str, err := m.GetOrOpenStream(4)
					Expect(err).ToNot(HaveOccurred())
					Expect(str).To(BeNil())
				})
			})

			Context("accepting streams", func() {
				It("accepts stream 2 first", func() {
					var str *stream
					go func() {
						defer GinkgoRecover()
						var err error
						str, err = m.AcceptStream()
						Expect(err).ToNot(HaveOccurred())
					}()
					_, err := m.GetOrOpenStream(2)
					Expect(err).ToNot(HaveOccurred())
					Eventually(func() *stream { return str }).ShouldNot(BeNil())
					Expect(str.StreamID()).To(Equal(protocol.StreamID(2)))
				})
			})
		})
	})

	Context("DoS mitigation, iterating and deleting", func() {
		BeforeEach(func() {
			setNewStreamsMap(protocol.PerspectiveServer)
		})

		closeStream := func(id protocol.StreamID) {
			str := m.streams[id]
			Expect(str).ToNot(BeNil())
			Expect(str.finished()).To(BeFalse())
			str.cancelled.Set(true)
			Expect(str.finished()).To(BeTrue())
		}

		Context("deleting streams", func() {
			BeforeEach(func() {
				for i := 1; i <= 5; i++ {
					err := m.putStream(&stream{streamID: protocol.StreamID(i)})
					Expect(err).ToNot(HaveOccurred())
				}
				Expect(m.openStreams).To(Equal([]protocol.StreamID{1, 2, 3, 4, 5}))
			})

			It("does not delete streams with Close()", func() {
				str, err := m.GetOrOpenStream(55)
				Expect(err).ToNot(HaveOccurred())
				str.Close()
				err = m.DeleteClosedStreams()
				Expect(err).ToNot(HaveOccurred())
				str, err = m.GetOrOpenStream(55)
				Expect(err).ToNot(HaveOccurred())
				Expect(str).ToNot(BeNil())
			})

			It("removes the first stream", func() {
				closeStream(1)
				err := m.DeleteClosedStreams()
				Expect(err).ToNot(HaveOccurred())
				Expect(m.openStreams).To(HaveLen(4))
				Expect(m.openStreams).To(Equal([]protocol.StreamID{2, 3, 4, 5}))
			})

			It("removes a stream in the middle", func() {
				closeStream(3)
				err := m.DeleteClosedStreams()
				Expect(err).ToNot(HaveOccurred())
				Expect(m.streams).To(HaveLen(4))
				Expect(m.openStreams).To(Equal([]protocol.StreamID{1, 2, 4, 5}))
			})

			It("removes a stream at the end", func() {
				closeStream(5)
				err := m.DeleteClosedStreams()
				Expect(err).ToNot(HaveOccurred())
				Expect(m.openStreams).To(HaveLen(4))
				Expect(m.openStreams).To(Equal([]protocol.StreamID{1, 2, 3, 4}))
			})

			It("removes all streams", func() {
				for i := 1; i <= 5; i++ {
					closeStream(protocol.StreamID(i))
				}
				err := m.DeleteClosedStreams()
				Expect(err).ToNot(HaveOccurred())
				Expect(m.streams).To(BeEmpty())
				Expect(m.openStreams).To(BeEmpty())
			})
		})

		Context("RoundRobinIterate", func() {
			// create 5 streams, ids 4 to 8
			var lambdaCalledForStream []protocol.StreamID
			var numIterations int

			BeforeEach(func() {
				lambdaCalledForStream = lambdaCalledForStream[:0]
				numIterations = 0
				for i := 4; i <= 8; i++ {
					err := m.putStream(&stream{streamID: protocol.StreamID(i)})
					Expect(err).NotTo(HaveOccurred())
				}
			})

			It("executes the lambda exactly once for every stream", func() {
				fn := func(str *stream) (bool, error) {
					lambdaCalledForStream = append(lambdaCalledForStream, str.StreamID())
					numIterations++
					return true, nil
				}
				err := m.RoundRobinIterate(fn)
				Expect(err).ToNot(HaveOccurred())
				Expect(numIterations).To(Equal(5))
				Expect(lambdaCalledForStream).To(Equal([]protocol.StreamID{4, 5, 6, 7, 8}))
				Expect(m.roundRobinIndex).To(BeZero())
			})

			It("goes around once when starting in the middle", func() {
				fn := func(str *stream) (bool, error) {
					lambdaCalledForStream = append(lambdaCalledForStream, str.StreamID())
					numIterations++
					return true, nil
				}
				m.roundRobinIndex = 3 // pointing to stream 7
				err := m.RoundRobinIterate(fn)
				Expect(err).ToNot(HaveOccurred())
				Expect(numIterations).To(Equal(5))
				Expect(lambdaCalledForStream).To(Equal([]protocol.StreamID{7, 8, 4, 5, 6}))
				Expect(m.roundRobinIndex).To(BeEquivalentTo(3))
			})

			It("picks up at the index+1 where it last stopped", func() {
				fn := func(str *stream) (bool, error) {
					lambdaCalledForStream = append(lambdaCalledForStream, str.StreamID())
					numIterations++
					if str.StreamID() == 5 {
						return false, nil
					}
					return true, nil
				}
				err := m.RoundRobinIterate(fn)
				Expect(err).ToNot(HaveOccurred())
				Expect(numIterations).To(Equal(2))
				Expect(lambdaCalledForStream).To(Equal([]protocol.StreamID{4, 5}))
				Expect(m.roundRobinIndex).To(BeEquivalentTo(2))
				numIterations = 0
				lambdaCalledForStream = lambdaCalledForStream[:0]
				fn2 := func(str *stream) (bool, error) {
					lambdaCalledForStream = append(lambdaCalledForStream, str.StreamID())
					numIterations++
					if str.StreamID() == 7 {
						return false, nil
					}
					return true, nil
				}
				err = m.RoundRobinIterate(fn2)
				Expect(err).ToNot(HaveOccurred())
				Expect(numIterations).To(Equal(2))
				Expect(lambdaCalledForStream).To(Equal([]protocol.StreamID{6, 7}))
			})

			Context("adjusting the RoundRobinIndex when deleting streams", func() {
				/*
					Index:	 	 	0  1  2  3  4
					StreamID:	[ 4, 5, 6, 7, 8 ]
				*/

				It("adjusts when deleting an element in front", func() {
					m.roundRobinIndex = 3 // stream 7
					deleteStream(5)
					Expect(m.roundRobinIndex).To(BeEquivalentTo(2))
				})

				It("doesn't adjust when deleting an element at the back", func() {
					m.roundRobinIndex = 1 // stream 5
					deleteStream(7)
					Expect(m.roundRobinIndex).To(BeEquivalentTo(1))
				})

				It("doesn't adjust when deleting the element it is pointing to", func() {
					m.roundRobinIndex = 3 // stream 7
					deleteStream(7)
					Expect(m.roundRobinIndex).To(BeEquivalentTo(3))
				})

				It("adjusts when deleting multiple elements", func() {
					m.roundRobinIndex = 3 // stream 7
					closeStream(5)
					closeStream(6)
					closeStream(8)
					err := m.DeleteClosedStreams()
					Expect(err).ToNot(HaveOccurred())
					Expect(m.roundRobinIndex).To(BeEquivalentTo(1))
				})
			})

			Context("Prioritizing crypto- and header streams", func() {
				BeforeEach(func() {
					err := m.putStream(&stream{streamID: 1})
					Expect(err).NotTo(HaveOccurred())
					err = m.putStream(&stream{streamID: 3})
					Expect(err).NotTo(HaveOccurred())
				})

				It("gets crypto- and header stream first, then picks up at the round-robin position", func() {
					m.roundRobinIndex = 3 // stream 7
					fn := func(str *stream) (bool, error) {
						if numIterations >= 3 {
							return false, nil
						}
						lambdaCalledForStream = append(lambdaCalledForStream, str.StreamID())
						numIterations++
						return true, nil
					}
					err := m.RoundRobinIterate(fn)
					Expect(err).ToNot(HaveOccurred())
					Expect(numIterations).To(Equal(3))
					Expect(lambdaCalledForStream).To(Equal([]protocol.StreamID{1, 3, 7}))
				})
			})
		})
	})
})
