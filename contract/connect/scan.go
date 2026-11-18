package connect

import (
	"bufio"
	"errors"
	"io"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// AmbiguousBodyError is a call whose JSON body could be read two ways at a pointer an operation
// matches on: an object names the key twice, or the key or the string there is not text every
// reader decodes alike. The broker passes such a body on as the app wrote it, so it sends none
// that another reader could take differently from what was matched.
type AmbiguousBodyError struct{ Pointer string }

func (e *AmbiguousBodyError) Error() string {
	return "the body can be read two ways at " + e.Pointer
}

// maxScanDepth is how deep a body's arrays and objects may nest, the depth encoding/json reads.
const maxScanDepth = 10000

// maxScanString is the longest key or string kept while scanning: anything longer equals no
// pointer token and no string an operation matches on (a pointer is 200 characters at most, a
// matched string MaxBodyMatch).
const maxScanString = 1024

// ScanBody reads r to its end as exactly one JSON document and returns the string at each of
// pointers that holds one, keyed by the pointer as written. It keeps nothing else of the body:
// what it needs is a fixed amount of memory whatever the body's size, so a body is checked as
// it streams past (BROKER.md §2, "Bodies").
//
// It is stricter than a decoder, because the bytes it passes are sent on unchanged and the
// outside system reads them with a parser of its own. A body is refused as ErrBodyNotJSON when
// it is not one RFC 8259 document in valid UTF-8, and as an AmbiguousBodyError when an object on
// the way to a pointer names that pointer's key twice, or a key or string there holds half of a
// surrogate pair. Keys repeated elsewhere are no part of what was matched and pass.
func ScanBody(r io.Reader, pointers []string) (map[string]string, error) {
	root := &scanNode{}
	for _, p := range pointers {
		toks, err := pointer(p)
		if err != nil {
			return nil, err
		}
		n := root
		for _, t := range toks {
			if n.children == nil {
				n.children = map[string]*scanNode{}
			}
			if n.children[t] == nil {
				n.children[t] = &scanNode{at: n.at + "/" + escapePointer(t)}
			}
			n = n.children[t]
		}
		n.leaf = p
	}
	s := &scanner{r: bufio.NewReaderSize(r, 64<<10), found: map[string]string{}}
	if err := s.value(root, 0); err != nil {
		return nil, err
	}
	_, err := s.next()
	if errors.Is(err, io.EOF) {
		return s.found, nil
	}
	if err == nil {
		return nil, ErrBodyNotJSON // something after the document
	}
	return nil, err
}

// scanNode is one place on the way to a pointer: its children by key or index, and the pointer
// that ends here.
type scanNode struct {
	children map[string]*scanNode
	leaf     string
	at       string
}

type scanner struct {
	r     *bufio.Reader
	found map[string]string
	buf   []byte
}

// errRead keeps a failed read of the body apart from a body that is not JSON.
type errRead struct{ err error }

func (e errRead) Error() string { return e.err.Error() }
func (e errRead) Unwrap() error { return e.err }

// byte reads one byte; the end of the body inside a value is not JSON.
func (s *scanner) byte() (byte, error) {
	c, err := s.r.ReadByte()
	if errors.Is(err, io.EOF) {
		return 0, ErrBodyNotJSON
	}
	if err != nil {
		return 0, errRead{err}
	}
	return c, nil
}

// next is the next byte that is not white space, or io.EOF at the body's end.
func (s *scanner) next() (byte, error) {
	for {
		c, err := s.r.ReadByte()
		if err == io.EOF {
			return 0, io.EOF
		}
		if err != nil {
			return 0, errRead{err}
		}
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return c, nil
		}
	}
}

// token is next inside a value, where the body's end is not JSON.
func (s *scanner) token() (byte, error) {
	c, err := s.next()
	if errors.Is(err, io.EOF) {
		return 0, ErrBodyNotJSON
	}
	return c, err
}

func (s *scanner) value(n *scanNode, depth int) error {
	c, err := s.token()
	if err != nil {
		return err
	}
	return s.valueFrom(c, n, depth)
}

func (s *scanner) valueFrom(c byte, n *scanNode, depth int) error {
	switch {
	case c == '{':
		return s.object(n, depth+1)
	case c == '[':
		return s.array(n, depth+1)
	case c == '"':
		keep := n != nil && n.leaf != ""
		v, whole, err := s.str(keep)
		if err != nil {
			return ambiguousAt(err, n)
		}
		if keep && whole {
			s.found[n.leaf] = v
		}
		return nil
	case c == 't':
		return s.literal("rue")
	case c == 'f':
		return s.literal("alse")
	case c == 'n':
		return s.literal("ull")
	case c == '-' || c >= '0' && c <= '9':
		return s.number(c)
	}
	return ErrBodyNotJSON
}

// errHalfPair is a string kept for matching that holds half of a surrogate pair.
var errHalfPair = errors.New("half of a surrogate pair")

func ambiguousAt(err error, n *scanNode) error {
	if errors.Is(err, errHalfPair) && n != nil {
		return &AmbiguousBodyError{Pointer: n.at}
	}
	return err
}

func (s *scanner) literal(rest string) error {
	for i := 0; i < len(rest); i++ {
		c, err := s.byte()
		if err != nil {
			return err
		}
		if c != rest[i] {
			return ErrBodyNotJSON
		}
	}
	return nil
}

// peek is the next byte without taking it; ok is false at the body's end.
func (s *scanner) peek() (byte, bool, error) {
	b, err := s.r.Peek(1)
	if err == io.EOF {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, errRead{err}
	}
	return b[0], true, nil
}

// digits takes a run of digits and reports how many.
func (s *scanner) digits() (int, error) {
	n := 0
	for {
		c, ok, err := s.peek()
		if err != nil {
			return n, err
		}
		if !ok || c < '0' || c > '9' {
			return n, nil
		}
		_, _ = s.r.ReadByte()
		n++
	}
}

// number takes a number whose first byte, c, is read: -?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?
func (s *scanner) number(c byte) error {
	if c == '-' {
		var err error
		if c, err = s.byte(); err != nil {
			return err
		}
		if c < '0' || c > '9' {
			return ErrBodyNotJSON
		}
	}
	if c != '0' {
		if _, err := s.digits(); err != nil {
			return err
		}
	}
	if c, ok, err := s.peek(); err != nil {
		return err
	} else if ok && c >= '0' && c <= '9' {
		return ErrBodyNotJSON // a digit after a leading zero
	}
	if err := s.fraction(); err != nil {
		return err
	}
	return s.exponent()
}

// fraction takes a number's fraction where there is one.
func (s *scanner) fraction() error {
	c, ok, err := s.peek()
	if err != nil || !ok || c != '.' {
		return err
	}
	_, _ = s.r.ReadByte()
	if n, err := s.digits(); err != nil || n == 0 {
		return orNotJSON(err)
	}
	return nil
}

// exponent takes a number's exponent where there is one.
func (s *scanner) exponent() error {
	c, ok, err := s.peek()
	if err != nil || !ok || (c != 'e' && c != 'E') {
		return err
	}
	_, _ = s.r.ReadByte()
	if c, ok, err = s.peek(); err != nil {
		return err
	}
	if ok && (c == '+' || c == '-') {
		_, _ = s.r.ReadByte()
	}
	if n, err := s.digits(); err != nil || n == 0 {
		return orNotJSON(err)
	}
	return nil
}

func orNotJSON(err error) error {
	if err != nil {
		return err
	}
	return ErrBodyNotJSON
}

func (s *scanner) object(n *scanNode, depth int) error {
	if depth > maxScanDepth {
		return ErrBodyNotJSON
	}
	seen := map[string]bool{}
	c, err := s.token()
	if err != nil {
		return err
	}
	if c == '}' {
		return nil
	}
	for {
		if c != '"' {
			return ErrBodyNotJSON
		}
		if err := s.member(n, seen, depth); err != nil {
			return err
		}
		if c, err = s.token(); err != nil {
			return err
		}
		if c == '}' {
			return nil
		}
		if c != ',' {
			return ErrBodyNotJSON
		}
		if c, err = s.token(); err != nil {
			return err
		}
	}
}

// member takes one key and its value, the key's opening quote read. A key the pointers follow
// may appear once.
func (s *scanner) member(n *scanNode, seen map[string]bool, depth int) error {
	tracked := n != nil && len(n.children) > 0
	key, whole, err := s.str(tracked)
	if err != nil {
		return ambiguousAt(err, n)
	}
	var child *scanNode
	if tracked && whole {
		child = n.children[key]
	}
	if child != nil {
		if seen[key] {
			return &AmbiguousBodyError{Pointer: child.at}
		}
		seen[key] = true
	}
	c, err := s.token()
	if err != nil {
		return err
	}
	if c != ':' {
		return ErrBodyNotJSON
	}
	return s.value(child, depth)
}

func (s *scanner) array(n *scanNode, depth int) error {
	if depth > maxScanDepth {
		return ErrBodyNotJSON
	}
	c, err := s.token()
	if err != nil {
		return err
	}
	if c == ']' {
		return nil
	}
	for i := 0; ; i++ {
		var child *scanNode
		if n != nil && len(n.children) > 0 {
			child = n.children[strconv.Itoa(i)]
		}
		if err := s.valueFrom(c, child, depth); err != nil {
			return err
		}
		if c, err = s.token(); err != nil {
			return err
		}
		if c == ']' {
			return nil
		}
		if c != ',' {
			return ErrBodyNotJSON
		}
		if c, err = s.token(); err != nil {
			return err
		}
	}
}

// str takes a string whose opening quote is read. With keep it returns the string decoded, and
// whole is false when it is longer than maxScanString (it then equals nothing matched); without
// keep it only checks it. Half of a surrogate pair is errHalfPair in a kept string and passes in
// any other, where no decision rests on it.
func (s *scanner) str(keep bool) (string, bool, error) {
	s.buf = s.buf[:0]
	whole := true
	add := func(p ...byte) {
		if !keep || !whole {
			return
		}
		if len(s.buf)+len(p) > maxScanString {
			whole = false
			return
		}
		s.buf = append(s.buf, p...)
	}
	for {
		c, err := s.byte()
		if err != nil {
			return "", false, err
		}
		switch {
		case c == '"':
			return string(s.buf), whole, nil
		case c == '\\':
			r, err := s.escape(keep)
			if err != nil {
				return "", false, err
			}
			add(utf8.AppendRune(nil, r)...)
		case c < 0x20:
			return "", false, ErrBodyNotJSON
		case c < utf8.RuneSelf:
			add(c)
		default:
			seq, err := s.utf8(c)
			if err != nil {
				return "", false, err
			}
			add(seq...)
		}
	}
}

// utf8 takes the rest of a UTF-8 sequence whose first byte is c, and refuses one that is not
// valid: readers differ in what they make of it.
func (s *scanner) utf8(c byte) ([]byte, error) {
	size := 0
	switch {
	case c&0xE0 == 0xC0:
		size = 2
	case c&0xF0 == 0xE0:
		size = 3
	case c&0xF8 == 0xF0:
		size = 4
	default:
		return nil, ErrBodyNotJSON
	}
	seq := make([]byte, 1, 4)
	seq[0] = c
	for len(seq) < size {
		b, err := s.byte()
		if err != nil {
			return nil, err
		}
		seq = append(seq, b)
	}
	if r, n := utf8.DecodeRune(seq); n != size || r == utf8.RuneError && n == 1 {
		return nil, ErrBodyNotJSON
	}
	return seq, nil
}

// escape takes an escape whose backslash is read and returns the character it names.
func (s *scanner) escape(keep bool) (rune, error) {
	c, err := s.byte()
	if err != nil {
		return 0, err
	}
	switch c {
	case '"', '\\', '/':
		return rune(c), nil
	case 'b':
		return '\b', nil
	case 'f':
		return '\f', nil
	case 'n':
		return '\n', nil
	case 'r':
		return '\r', nil
	case 't':
		return '\t', nil
	case 'u':
	default:
		return 0, ErrBodyNotJSON
	}
	r, err := s.hex4()
	if err != nil {
		return 0, err
	}
	if !utf16.IsSurrogate(r) {
		return r, nil
	}
	// A high half is one character with the low half that follows it; anything else is half a
	// pair, which each reader replaces or refuses in its own way.
	if b, err := s.r.Peek(2); err == nil && b[0] == '\\' && b[1] == 'u' {
		if b6, err := s.r.Peek(6); err == nil {
			if lo, ok := hex4(b6[2:]); ok {
				if full := utf16.DecodeRune(r, lo); full != utf8.RuneError {
					_, _ = s.r.Discard(6)
					return full, nil
				}
			}
		}
	}
	if keep {
		return 0, errHalfPair
	}
	return utf8.RuneError, nil
}

func (s *scanner) hex4() (rune, error) {
	var b [4]byte
	for i := range b {
		c, err := s.byte()
		if err != nil {
			return 0, err
		}
		b[i] = c
	}
	r, ok := hex4(b[:])
	if !ok {
		return 0, ErrBodyNotJSON
	}
	return r, nil
}

func hex4(b []byte) (rune, bool) {
	var r rune
	for _, c := range b[:4] {
		switch {
		case c >= '0' && c <= '9':
			c -= '0'
		case c >= 'a' && c <= 'f':
			c = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			c = c - 'A' + 10
		default:
			return 0, false
		}
		r = r<<4 | rune(c)
	}
	return r, true
}
