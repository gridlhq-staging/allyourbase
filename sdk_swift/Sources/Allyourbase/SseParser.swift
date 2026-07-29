import Foundation

public struct SseMessage {
    public let event: String?
    public let data: String?
    public let id: String?
    public let retry: Int?

    public init(event: String? = nil, data: String? = nil, id: String? = nil, retry: Int? = nil) {
        self.event = event
        self.data = data
        self.id = id
        self.retry = retry
    }
}

public struct SseParser<Bytes: AsyncSequence> where Bytes.Element == UInt8, Bytes: Sendable {
    private let bytes: Bytes

    public init(bytes: Bytes) {
        self.bytes = bytes
    }

    public func messages() -> AsyncThrowingStream<SseMessage, Error> {
        let inputBytes = bytes
        return AsyncThrowingStream<SseMessage, Error> { continuation in
            Task {
                do {
                    var buffer: [UInt8] = []
                    var currentEvent: String?
                    var currentData: String?
                    var currentId: String?
                    var currentRetry: Int?
                    var hasField = false

                    func flush() {
                        guard hasField else {
                            return
                        }
                        continuation.yield(
                            SseMessage(
                                event: currentEvent,
                                data: currentData,
                                id: currentId,
                                retry: currentRetry
                            )
                        )
                        currentEvent = nil
                        currentData = nil
                        currentId = nil
                        currentRetry = nil
                        hasField = false
                    }

                    func process(line: String) {
                        if line.isEmpty {
                            flush()
                            return
                        }
                        if line.hasPrefix(":") {
                            return
                        }
                        guard let separator = line.firstIndex(of: ":") else {
                            return
                        }

                        let field = String(line[..<separator])
                        var value = String(line[line.index(after: separator)...])
                        if value.hasPrefix(" ") {
                            value.removeFirst()
                        }

                        switch field {
                        case "event":
                            currentEvent = value
                            hasField = true
                        case "data":
                            if let existing = currentData {
                                currentData = "\(existing)\n\(value)"
                            } else {
                                currentData = value
                            }
                            hasField = true
                        case "id":
                            currentId = value
                            hasField = true
                        case "retry":
                            if let retry = Int(value) {
                                currentRetry = retry
                                hasField = true
                            }
                        default:
                            break
                        }
                    }

                    for try await byte in inputBytes {
                        if byte == 10 {
                            process(line: String(decoding: buffer, as: UTF8.self))
                            buffer.removeAll(keepingCapacity: true)
                        } else if byte != 13 {
                            buffer.append(byte)
                        }
                    }

                    if !buffer.isEmpty {
                        process(line: String(decoding: buffer, as: UTF8.self))
                    }
                    flush()
                    continuation.finish()
                } catch is CancellationError {
                    continuation.finish()
                } catch {
                    continuation.finish(throwing: error)
                }
            }
        }
    }
}
