package mailer

import "io"

type base64LineWriter struct {
	w io.Writer
	n int
}

func newBase64LineWriter(w io.Writer) *base64LineWriter {
	return &base64LineWriter{w: w}
}

func (w *base64LineWriter) Write(p []byte) (int, error) {
	total := 0
	for _, b := range p {
		if w.n == 76 {
			if _, err := w.w.Write([]byte("\r\n")); err != nil {
				return total, err
			}
			w.n = 0
		}

		if _, err := w.w.Write([]byte{b}); err != nil {
			return total, err
		}

		total++
		w.n++
	}

	return total, nil
}
