package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// object is a JSON object that keeps its key order and leaves every value it does not touch as the
// raw bytes it was read with.
type object struct {
	keys []string
	vals map[string]json.RawMessage
}

func newObject() *object { return &object{vals: map[string]json.RawMessage{}} }

// parseObject decodes b, which must hold exactly one JSON object. Duplicate keys keep the first
// position and the last value, as encoding/json would.
func parseObject(b []byte) (*object, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, errors.New("not a JSON object")
	}
	o := newObject()
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		o.set(t.(string), v)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("trailing data after the JSON object")
	}
	return o, nil
}

func (o *object) get(k string) (json.RawMessage, bool) {
	v, ok := o.vals[k]
	return v, ok && string(v) != "null"
}

func (o *object) set(k string, v json.RawMessage) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func (o *object) del(k string) {
	if _, ok := o.vals[k]; !ok {
		return
	}
	delete(o.vals, k)
	for i, kk := range o.keys {
		if kk == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			return
		}
	}
}

// child returns the object at k, a new empty one when k is absent or null.
func (o *object) child(k string) (*object, error) {
	v, ok := o.get(k)
	if !ok {
		return newObject(), nil
	}
	c, err := parseObject(v)
	if err != nil {
		return nil, fmt.Errorf("%q: %w", k, err)
	}
	return c, nil
}

// array returns the array at k, nil when k is absent or null.
func (o *object) array(k string) ([]json.RawMessage, error) {
	v, ok := o.get(k)
	if !ok {
		return nil, nil
	}
	var a []json.RawMessage
	if err := json.Unmarshal(v, &a); err != nil {
		return nil, fmt.Errorf("%q is not an array", k)
	}
	return a, nil
}

// raw encodes o compactly, values verbatim.
func (o *object) raw() json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(marshal(k))
		b.WriteByte(':')
		b.Write(o.vals[k])
	}
	b.WriteByte('}')
	return b.Bytes()
}

// marshal encodes v without HTML escaping, so commands keep their `<`, `>` and `&`.
func marshal(v any) json.RawMessage {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(v) // values here are strings, slices and plain structs; Encode cannot fail
	return bytes.TrimRight(b.Bytes(), "\n")
}

// rawArray encodes a as a compact JSON array.
func rawArray(a []json.RawMessage) json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('[')
	for i, v := range a {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(v)
	}
	b.WriteByte(']')
	return b.Bytes()
}
