//go:build awsnitro

package nitro

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

const (
	ckrOK                         = 0x000
	ckrGeneralError               = 0x005
	ckrArgumentsBad               = 0x007
	ckrDeviceError                = 0x030
	ckrKeyHandleInvalid           = 0x060
	ckrMechanismInvalid           = 0x070
	ckrObjectHandleInvalid        = 0x082
	ckrOperationNotInitialized    = 0x091
	ckrPinIncorrect               = 0x0a0
	ckrSessionHandleInvalid       = 0x0b3
	ckrUserAlreadyLoggedIn        = 0x100
	ckrUserNotLoggedIn            = 0x101
	ckrBufferTooSmall             = 0x150
	ckrCryptokiNotInitialized     = 0x190
	ckrCryptokiAlreadyInitialized = 0x191

	ckfRWSession     = 0x002
	ckfSerialSession = 0x004
	ckfLoginRequired = 0x004

	ckuUser = 1

	ckaClass = 0x000
	ckaLabel = 0x003

	ckoPrivateKey = 3

	ckmRSAPKCS    = 0x0001
	ckmRSAPKCSPSS = 0x000d
	ckmSHA1       = 0x0220
	ckmSHA224     = 0x0255
	ckmSHA256     = 0x0250
	ckmSHA384     = 0x0260
	ckmSHA512     = 0x0270
	ckmECDSA      = 0x1041

	ckgMGF1SHA1   = 1
	ckgMGF1SHA256 = 2
	ckgMGF1SHA384 = 3
	ckgMGF1SHA512 = 4
	ckgMGF1SHA224 = 5
)

// p11-kit RPC call identifiers, see p11-kit/rpc-message.h.
const (
	callError             = 0
	callInitialize        = 1
	callFinalize          = 2
	callGetSlotList       = 4
	callGetTokenInfo      = 6
	callOpenSession       = 10
	callCloseSession      = 11
	callLogin             = 18
	callFindObjectsInit   = 26
	callFindObjects       = 27
	callFindObjectsFinal  = 28
	callSignInit          = 42
	callSign              = 43
	rpcProtocolVersion    = 0
	rpcHandshake          = "PRIVATE-GNOME-KEYRING-PKCS11-PROTOCOL-V-1"
	maxRPCMessageSize     = 1 << 20
	nullByteArrayLength   = 0xffffffff
	maxSignatureLength    = 1024
	maxFindObjectsResults = 16
)

// p11-kit RPC call signatures, see p11_rpc_calls in p11-kit/rpc-message.h.
var callSignatures = map[uint32][2]string{
	callError:            {"", "u"},
	callInitialize:       {"ayyay", ""},
	callFinalize:         {"", ""},
	callGetSlotList:      {"yfu", "au"},
	callGetTokenInfo:     {"u", "ssssuuuuuuuuuuuvvs"},
	callOpenSession:      {"uu", "u"},
	callCloseSession:     {"u", ""},
	callLogin:            {"uuay", ""},
	callFindObjectsInit:  {"uaA", ""},
	callFindObjects:      {"ufu", "au"},
	callFindObjectsFinal: {"u", ""},
	callSignInit:         {"uMu", ""},
	callSign:             {"uayfy", "ay"},
}

var errProtocol = errors.New("p11-kit rpc protocol error")

type ckError uint64

func (e ckError) Error() string {
	names := map[ckError]string{
		ckrGeneralError:               "CKR_GENERAL_ERROR",
		ckrArgumentsBad:               "CKR_ARGUMENTS_BAD",
		ckrDeviceError:                "CKR_DEVICE_ERROR",
		ckrKeyHandleInvalid:           "CKR_KEY_HANDLE_INVALID",
		ckrMechanismInvalid:           "CKR_MECHANISM_INVALID",
		ckrObjectHandleInvalid:        "CKR_OBJECT_HANDLE_INVALID",
		ckrOperationNotInitialized:    "CKR_OPERATION_NOT_INITIALIZED",
		ckrPinIncorrect:               "CKR_PIN_INCORRECT",
		ckrSessionHandleInvalid:       "CKR_SESSION_HANDLE_INVALID",
		ckrUserAlreadyLoggedIn:        "CKR_USER_ALREADY_LOGGED_IN",
		ckrUserNotLoggedIn:            "CKR_USER_NOT_LOGGED_IN",
		ckrBufferTooSmall:             "CKR_BUFFER_TOO_SMALL",
		ckrCryptokiNotInitialized:     "CKR_CRYPTOKI_NOT_INITIALIZED",
		ckrCryptokiAlreadyInitialized: "CKR_CRYPTOKI_ALREADY_INITIALIZED",
	}
	if n, ok := names[e]; ok {
		return "pkcs11: " + n
	}
	return fmt.Sprintf("pkcs11: CKR 0x%x", uint64(e))
}

type (
	attribute struct {
		typ   uint32
		ulong bool
		value []byte
		num   uint64
	}

	mechanism struct {
		typ uint32
		pss *pssParams
	}

	pssParams struct {
		hashAlg, mgf, sLen uint64
	}

	tokenInfo struct {
		label string
		flags uint64
	}

	encoder struct {
		b []byte
	}

	decoder struct {
		b   []byte
		err error
	}
)

func ulongAttribute(typ uint32, v uint64) attribute {
	return attribute{typ: typ, ulong: true, num: v}
}

func bytesAttribute(typ uint32, v []byte) attribute {
	return attribute{typ: typ, value: v}
}

func newRequest(call uint32) *encoder {
	e := &encoder{}
	e.uint32(call)
	e.rawByteArray([]byte(callSignatures[call][0]))
	return e
}

func (e *encoder) byte(v byte) { e.b = append(e.b, v) }

func (e *encoder) uint32(v uint32) { e.b = binary.BigEndian.AppendUint32(e.b, v) }

// ulong encodes "u", CK_ULONG is always transferred as 64 bit.
func (e *encoder) ulong(v uint64) { e.b = binary.BigEndian.AppendUint64(e.b, v) }

// rawByteArray encodes p11_rpc_buffer_add_byte_array.
func (e *encoder) rawByteArray(v []byte) {
	if v == nil {
		e.uint32(nullByteArrayLength)
		return
	}
	e.uint32(uint32(len(v)))
	e.b = append(e.b, v...)
}

// byteArray encodes "ay", see p11_rpc_message_write_byte_array.
func (e *encoder) byteArray(v []byte) {
	e.byte(1)
	e.rawByteArray(v)
}

// buffer encodes "fy" and "fu" as the capacity of the output buffer.
func (e *encoder) buffer(n uint32) { e.uint32(n) }

// attributes encodes "aA", see p11_rpc_buffer_add_attribute.
func (e *encoder) attributes(attrs []attribute) {
	e.uint32(uint32(len(attrs)))
	for _, a := range attrs {
		e.uint32(a.typ)
		e.byte(1)
		if a.ulong {
			e.uint32(8)
			e.ulong(a.num)
		} else {
			e.uint32(uint32(len(a.value)))
			e.rawByteArray(a.value)
		}
	}
}

// mechanism encodes "M", see p11_rpc_buffer_add_mechanism.
func (e *encoder) mechanism(m mechanism) {
	e.uint32(m.typ)
	switch m.typ {
	case ckmRSAPKCS, ckmECDSA:
		return
	}
	if m.pss == nil {
		e.byte(0)
		return
	}
	e.byte(1)
	e.ulong(m.pss.hashAlg)
	e.ulong(m.pss.mgf)
	e.ulong(m.pss.sLen)
}

func (d *decoder) fail(format string, args ...any) {
	if d.err == nil {
		d.err = fmt.Errorf("%w: "+format, append([]any{errProtocol}, args...)...)
	}
}

func (d *decoder) next(n int) []byte {
	if d.err != nil {
		return nil
	}
	if n < 0 || len(d.b) < n {
		d.fail("short message")
		return nil
	}
	v := d.b[:n:n]
	d.b = d.b[n:]
	return v
}

func (d *decoder) byte() byte {
	if v := d.next(1); v != nil {
		return v[0]
	}
	return 0
}

func (d *decoder) uint32() uint32 {
	if v := d.next(4); v != nil {
		return binary.BigEndian.Uint32(v)
	}
	return 0
}

func (d *decoder) ulong() uint64 {
	if v := d.next(8); v != nil {
		return binary.BigEndian.Uint64(v)
	}
	return 0
}

func (d *decoder) rawByteArray() []byte {
	n := d.uint32()
	if d.err != nil || n == nullByteArrayLength {
		return nil
	}
	if n >= 0x7fffffff {
		d.fail("invalid byte array length %d", n)
		return nil
	}
	return d.next(int(n))
}

// byteArray decodes "ay". If no data is present, the returned length
// is the size the caller needs to provide.
func (d *decoder) byteArray() ([]byte, uint32) {
	if d.byte() == 0 {
		return nil, d.uint32()
	}
	v := d.rawByteArray()
	return v, uint32(len(v))
}

// ulongArray decodes "au". If no data is present, the returned length
// is the size the caller needs to provide.
func (d *decoder) ulongArray() ([]uint64, uint32) {
	valid := d.byte()
	n := d.uint32()
	if valid == 0 || d.err != nil {
		return nil, n
	}
	if uint64(n)*8 > uint64(len(d.b)) {
		d.fail("invalid ulong array length %d", n)
		return nil, 0
	}
	v := make([]uint64, n)
	for i := range v {
		v[i] = d.ulong()
	}
	return v, n
}

func (d *decoder) spaceString() string {
	return strings.TrimRight(string(d.rawByteArray()), " \x00")
}

func (d *decoder) version() {
	d.next(2)
}

func (d *decoder) tokenInfo() tokenInfo {
	var ti tokenInfo
	ti.label = d.spaceString()
	d.spaceString() // manufacturerID
	d.spaceString() // model
	d.spaceString() // serialNumber
	ti.flags = d.ulong()
	for range 10 {
		d.ulong()
	}
	d.version() // hardwareVersion
	d.version() // firmwareVersion
	d.spaceString()
	return ti
}

// attributes decodes "aA" for byte array and ulong values.
func (d *decoder) attributes() []attribute {
	n := d.uint32()
	if d.err != nil {
		return nil
	}
	if uint64(n)*5 > uint64(len(d.b)) {
		d.fail("invalid attribute array length %d", n)
		return nil
	}
	attrs := make([]attribute, 0, n)
	for range n {
		a := attribute{typ: d.uint32()}
		if d.byte() == 0 {
			attrs = append(attrs, a)
			continue
		}
		d.uint32() // length
		if a.typ == ckaClass {
			a.ulong = true
			a.num = d.ulong()
		} else {
			a.value = d.rawByteArray()
		}
		attrs = append(attrs, a)
	}
	return attrs
}

// mechanism decodes "M" for the mechanisms used by this package.
func (d *decoder) mechanism() mechanism {
	m := mechanism{typ: d.uint32()}
	switch m.typ {
	case ckmRSAPKCS, ckmECDSA:
		return m
	case ckmRSAPKCSPSS:
		if d.byte() == 1 {
			m.pss = &pssParams{hashAlg: d.ulong(), mgf: d.ulong(), sLen: d.ulong()}
		}
		return m
	default:
		if d.byte() == 1 {
			d.rawByteArray()
		}
		return m
	}
}

func (d *decoder) done() error {
	if d.err == nil && len(d.b) != 0 {
		d.fail("%d trailing bytes", len(d.b))
	}
	return d.err
}
