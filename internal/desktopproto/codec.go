package desktopproto

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Decoder reads bounded, newline-terminated JSON objects. It is not safe for
// concurrent use. The launcher should additionally put a deadline on its
// startup pipe before waiting for starting/ready records.
type Decoder struct {
	r *bufio.Reader
}

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReader(r)}
}

// ReadHelper reads and validates one helper-to-launcher record. Unknown
// additive JSON fields are ignored for forward compatibility.
func (d *Decoder) ReadHelper() (HelperRecord, error) {
	line, err := d.readLine()
	if err != nil {
		return HelperRecord{}, err
	}
	if err := validateJSONEnvelope(line); err != nil {
		return HelperRecord{}, err
	}
	var record HelperRecord
	if err := json.Unmarshal(line, &record); err != nil {
		return HelperRecord{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if err := record.Validate(); err != nil {
		return HelperRecord{}, err
	}
	return record, nil
}

// ReadLauncher reads and validates one launcher-to-helper record. Unknown
// additive JSON fields are ignored for forward compatibility.
func (d *Decoder) ReadLauncher() (LauncherRecord, error) {
	line, err := d.readLine()
	if err != nil {
		return LauncherRecord{}, err
	}
	if err := validateJSONEnvelope(line); err != nil {
		return LauncherRecord{}, err
	}
	var record LauncherRecord
	if err := json.Unmarshal(line, &record); err != nil {
		return LauncherRecord{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if err := record.Validate(); err != nil {
		return LauncherRecord{}, err
	}
	return record, nil
}

// EncodeHelper writes exactly one validated helper record and terminating
// newline. Human-readable logs must use stderr instead of this writer.
func EncodeHelper(w io.Writer, record HelperRecord) error {
	if err := record.Validate(); err != nil {
		return err
	}
	return encode(w, record)
}

// EncodeLauncher writes exactly one validated launcher request and terminating
// newline.
func EncodeLauncher(w io.Writer, record LauncherRecord) error {
	if err := record.Validate(); err != nil {
		return err
	}
	return encode(w, record)
}

func encode(w io.Writer, record any) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("desktopproto: encode: %w", err)
	}
	if len(data) > MaxRecordBytes {
		return ErrRecordTooLarge
	}
	data = append(data, '\n')
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func (d *Decoder) readLine() ([]byte, error) {
	var line []byte
	for {
		fragment, err := d.r.ReadSlice('\n')
		line = append(line, fragment...)
		if len(line) > MaxRecordBytes+1 {
			return nil, ErrRecordTooLarge
		}
		switch {
		case err == nil:
			line = bytes.TrimSuffix(line, []byte{'\n'})
			if len(line) > MaxRecordBytes {
				return nil, ErrRecordTooLarge
			}
			if len(bytes.TrimSpace(line)) == 0 {
				return nil, ErrMalformed
			}
			return line, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			if len(line) == 0 {
				return nil, io.EOF
			}
			return nil, ErrTruncated
		default:
			return nil, err
		}
	}
}

func validateJSONEnvelope(line []byte) error {
	var envelope struct {
		V    int  `json:"v"`
		Type Type `json:"type"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	return validateEnvelope(envelope.V, envelope.Type)
}
