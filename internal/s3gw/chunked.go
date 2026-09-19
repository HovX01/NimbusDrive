package s3gw

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// awsChunkReader decodes the AWS "aws-chunked" transfer encoding that S3 clients
// use for streaming uploads. Each chunk is framed as
// "<hex-size>;chunk-signature=<sig>\r\n<data>\r\n", terminated by a zero-size chunk.
// ponytail: per-chunk signatures are not verified (the outer SigV4 signature still
// authenticates the request); add chunk-signature validation if tamper evidence
// over an untrusted transport matters.
type awsChunkReader struct {
	r   *bufio.Reader
	buf []byte
	eof bool
}

func newAWSChunkReader(r io.Reader) *awsChunkReader {
	return &awsChunkReader{r: bufio.NewReaderSize(r, 64<<10)}
}

var _ io.Reader = (*awsChunkReader)(nil)

func (c *awsChunkReader) Read(p []byte) (int, error) {
	if c.eof {
		return 0, io.EOF
	}
	if len(c.buf) == 0 {
		if err := c.nextChunk(); err != nil {
			return 0, err
		}
		if c.eof {
			return 0, io.EOF
		}
	}
	n := copy(p, c.buf)
	c.buf = c.buf[n:]
	return n, nil
}

func (c *awsChunkReader) nextChunk() error {
	line, err := c.r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("aws-chunked: read chunk header: %w", err)
	}
	sizeField, _, _ := strings.Cut(strings.TrimRight(line, "\r\n"), ";")
	size, err := strconv.ParseInt(strings.TrimSpace(sizeField), 16, 64)
	if err != nil {
		return fmt.Errorf("aws-chunked: bad chunk size %q: %w", line, err)
	}
	if size == 0 {
		// Final chunk is followed by a trailing CRLF.
		_, _ = c.r.Discard(2)
		c.eof = true
		return nil
	}
	chunk := make([]byte, size)
	if _, err := io.ReadFull(c.r, chunk); err != nil {
		return fmt.Errorf("aws-chunked: short chunk: %w", err)
	}
	_, _ = c.r.Discard(2)
	c.buf = chunk
	return nil
}

// chunkedBody returns the request body, unwrapping aws-chunked framing when the
// client used a streaming signature.
func chunkedBody(r *http.Request) io.Reader {
	if isAWSChunked(r) {
		return newAWSChunkReader(r.Body)
	}
	return r.Body
}

func isAWSChunked(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "aws-chunked") {
		return true
	}
	return strings.HasPrefix(strings.ToUpper(r.Header.Get("X-Amz-Content-Sha256")), "STREAMING")
}
