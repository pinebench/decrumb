package cli

import (
	"bufio"
	"bytes"
	"errors"
	"io"
)

// maxLineLen is the longest input line accepted, in bytes, not counting the
// line ending: 2 MB, Chrome's maximum URL length (spec §5).
const maxLineLen = 2 << 20

// lineReader reads stdin line by line, with a limit on the line length.
// Unlike bufio.Scanner, which stops for good at a line that is too long, it
// skips that line and carries on with the next one.
type lineReader struct {
	r      *bufio.Reader
	maxLen int
}

func newLineReader(r io.Reader, maxLen int) *lineReader {
	return &lineReader{r: bufio.NewReaderSize(r, 64*1024), maxLen: maxLen}
}

// next returns the next line without its "\n" or "\r\n". A line longer than
// maxLen bytes is read to its end and dropped: next returns tooLong = true
// and no line, and the following call starts on the next line. At the end of
// the input, err is io.EOF.
func (lr *lineReader) next() (line []byte, tooLong bool, err error) {
	for {
		// ReadSlice returns at most one buffer's worth at a time
		// (ErrBufferFull), so a huge line never has to fit in memory.
		chunk, err := lr.r.ReadSlice('\n')
		if !tooLong {
			line = append(line, chunk...)
			// The line ending adds at most 2 bytes ("\r\n").
			if len(line) > lr.maxLen+2 {
				tooLong, line = true, nil
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) {
			if len(line) == 0 && !tooLong {
				return nil, false, io.EOF
			}
			break // last line, without a line ending
		}
		if err != nil {
			return nil, false, err
		}
		break
	}

	if tooLong {
		return nil, true, nil
	}
	line = bytes.TrimSuffix(line, []byte("\n"))
	line = bytes.TrimSuffix(line, []byte("\r"))
	if len(line) > lr.maxLen {
		return nil, true, nil
	}
	return line, false, nil
}
