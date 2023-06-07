package gopacketntp

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"example.com/scion-time/net/ntske"
	"github.com/secure-io/siv-go"
)

const (
	NumStoredCookies int = 8
)

const (
	extUniqueIdentifier  uint16 = 0x104
	extCookie            uint16 = 0x204
	extCookiePlaceholder uint16 = 0x304
	extAuthenticator     uint16 = 0x404
)

func (p *Packet) InitNTSRequestPacket(ntskeData ntske.Data) {
	var uid UniqueIdentifier
	uid.Generate()
	p.UniqueID = uid

	var cookie Cookie
	cookie.Cookie = ntskeData.Cookie[0]
	p.Cookies = append(p.Cookies, cookie)

	// Add cookie extension fields here s.t. 8 cookies are available after response.
	cookiePlaceholderData := make([]byte, len(cookie.Cookie))
	for i := len(ntskeData.Cookie); i < NumStoredCookies; i++ {
		var cookiePlacholder CookiePlaceholder
		cookiePlacholder.Cookie = cookiePlaceholderData
		p.CookiePlaceholders = append(p.CookiePlaceholders, cookiePlacholder)
	}

	var auth Authenticator
	auth.Key = ntskeData.C2sKey
	p.Auth = auth

	p.NTSMode = true
}

func (p *Packet) ProcessResponse(ntskeFetcher *ntske.Fetcher, reqID []byte, key []byte) error {
	err := p.Authenticate(key)
	if err != nil {
		return err
	}

	if !bytes.Equal(reqID, p.UniqueID.ID) {
		return errors.New("unexpected response ID")
	}
	for _, cookie := range p.Cookies {
		ntskeFetcher.StoreCookie(cookie.Cookie)
	}
	return nil
}

func (p *Packet) Authenticate(key []byte) error {
	if p.Auth.CipherText == nil {
		return errors.New("packet does not contain a valid authenticator")
	}
	if p.UniqueID.ID == nil {
		return errors.New("packet does not contain a unique identifier")
	}

	aessiv, err := siv.NewCMAC(key)
	if err != nil {
		return err
	}

	decrytedBuf, err := aessiv.Open(nil, p.Auth.Nonce, p.Auth.CipherText, p.BaseLayer.Contents[:p.Auth.Pos])
	if err != nil {
		return err
	}

	msgbuf := bytes.NewReader(decrytedBuf)
	for msgbuf.Len() >= 28 {
		var eh ExtHdr
		err := eh.unpack(msgbuf)
		if err != nil {
			return fmt.Errorf("unpack extension field: %s", err)
		}

		switch eh.Type {
		case extCookie:
			cookie := Cookie{ExtHdr: eh}
			err = cookie.unpack(msgbuf)
			if err != nil {
				return fmt.Errorf("unpack Cookie: %s", err)
			}
			p.Cookies = append(p.Cookies, cookie)

		default:
			// Unknown extension field. Skip it.
			_, err := msgbuf.Seek(int64(eh.Length), io.SeekCurrent)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *Packet) InitNTSResponsePacket(cookies [][]byte, key []byte, uniqueid []byte) {
	var uid UniqueIdentifier
	uid.ID = uniqueid
	p.UniqueID = uid

	buf := new(bytes.Buffer)
	for _, c := range cookies {
		var cookie Cookie
		cookie.Cookie = c
		cookie.pack(buf)
	}

	var auth Authenticator
	auth.Key = key
	auth.AssociatedData = buf.Bytes()
	p.Auth = auth

	p.NTSMode = true
}

type ExtHdr struct {
	Type   uint16
	Length uint16
}

func (h ExtHdr) pack(buf *bytes.Buffer) error {
	err := binary.Write(buf, binary.BigEndian, h)
	return err
}

func (h *ExtHdr) unpack(buf *bytes.Reader) error {
	err := binary.Read(buf, binary.BigEndian, h)
	return err
}

func (h ExtHdr) Header() ExtHdr { return h }

type UniqueIdentifier struct {
	ExtHdr
	ID []byte
}

func (u UniqueIdentifier) pack(buf *bytes.Buffer) error {
	value := new(bytes.Buffer)
	err := binary.Write(value, binary.BigEndian, u.ID)
	if err != nil {
		return err
	}
	if value.Len() < 32 {
		return fmt.Errorf("UniqueIdentifier.ID < 32 bytes")
	}

	newlen := (value.Len() + 3) & ^3
	padding := make([]byte, newlen-value.Len())

	u.ExtHdr.Type = extUniqueIdentifier
	u.ExtHdr.Length = 4 + uint16(newlen)
	err = u.ExtHdr.pack(buf)
	if err != nil {
		return err
	}

	_, err = buf.ReadFrom(value)
	if err != nil {
		return err
	}

	_, err = buf.Write(padding)
	if err != nil {
		return err
	}

	return nil
}

func (u *UniqueIdentifier) unpack(buf *bytes.Reader) error {
	if u.ExtHdr.Type != extUniqueIdentifier {
		return fmt.Errorf("expected unpacked EF header")
	}
	valueLen := u.ExtHdr.Length - uint16(binary.Size(u.ExtHdr))
	id := make([]byte, valueLen)
	if err := binary.Read(buf, binary.BigEndian, id); err != nil {
		return err
	}
	u.ID = id
	return nil
}

func (u *UniqueIdentifier) Generate() ([]byte, error) {
	id := make([]byte, 32)

	_, err := rand.Read(id)
	if err != nil {
		return nil, err
	}

	u.ID = id

	return id, nil
}

type Cookie struct {
	ExtHdr
	Cookie []byte
}

func (c Cookie) pack(buf *bytes.Buffer) error {
	value := new(bytes.Buffer)
	origlen, err := value.Write(c.Cookie)
	if err != nil {
		return err
	}

	// Round up to nearest word boundary
	newlen := (origlen + 3) & ^3
	padding := make([]byte, newlen-origlen)

	c.ExtHdr.Type = extCookie
	c.ExtHdr.Length = 4 + uint16(newlen)
	err = c.ExtHdr.pack(buf)
	if err != nil {
		return err
	}

	_, err = buf.ReadFrom(value)
	if err != nil {
		return err
	}
	_, err = buf.Write(padding)
	if err != nil {
		return err
	}

	return nil
}

func (c *Cookie) unpack(buf *bytes.Reader) error {
	if c.ExtHdr.Type != extCookie {
		return fmt.Errorf("expected unpacked EF header")
	}
	valueLen := c.ExtHdr.Length - uint16(binary.Size(c.ExtHdr))
	cookie := make([]byte, valueLen)
	if err := binary.Read(buf, binary.BigEndian, cookie); err != nil {
		return err
	}
	c.Cookie = cookie
	return nil
}

type CookiePlaceholder struct {
	ExtHdr
	Cookie []byte
}

func (c CookiePlaceholder) pack(buf *bytes.Buffer) error {
	value := new(bytes.Buffer)
	origlen, err := value.Write(c.Cookie)
	if err != nil {
		return err
	}

	// Round up to nearest word boundary
	newlen := (origlen + 3) & ^3
	padding := make([]byte, newlen-origlen)

	c.ExtHdr.Type = extCookiePlaceholder
	c.ExtHdr.Length = 4 + uint16(newlen)
	err = c.ExtHdr.pack(buf)
	if err != nil {
		return err
	}

	_, err = buf.ReadFrom(value)
	if err != nil {
		return err
	}
	_, err = buf.Write(padding)
	if err != nil {
		return err
	}

	return nil
}

type Key []byte

type Authenticator struct {
	ExtHdr
	NonceLen       uint16
	CipherTextLen  uint16
	Nonce          []byte
	AssociatedData []byte
	CipherText     []byte
	Key            Key
	Pos            int
}

func (a Authenticator) pack(buf *bytes.Buffer) error {
	aessiv, err := siv.NewCMAC(a.Key)
	if err != nil {
		return err
	}

	bits := make([]byte, 16)
	_, err = rand.Read(bits)
	if err != nil {
		return err
	}

	a.Nonce = bits

	a.CipherText = aessiv.Seal(nil, a.Nonce, a.AssociatedData, buf.Bytes())
	a.CipherTextLen = uint16(len(a.CipherText))

	noncebuf := new(bytes.Buffer)
	err = binary.Write(noncebuf, binary.BigEndian, a.Nonce)
	if err != nil {
		return err
	}
	a.NonceLen = uint16(noncebuf.Len())

	cipherbuf := new(bytes.Buffer)
	err = binary.Write(cipherbuf, binary.BigEndian, a.CipherText)
	if err != nil {
		return err
	}
	a.CipherTextLen = uint16(cipherbuf.Len())

	extbuf := new(bytes.Buffer)

	err = binary.Write(extbuf, binary.BigEndian, a.NonceLen)
	if err != nil {
		return err
	}

	err = binary.Write(extbuf, binary.BigEndian, a.CipherTextLen)
	if err != nil {
		return err
	}

	_, err = extbuf.ReadFrom(noncebuf)
	if err != nil {
		return err
	}
	noncepadding := make([]byte, (noncebuf.Len()+3) & ^3)
	_, err = extbuf.Write(noncepadding)
	if err != nil {
		return err
	}

	_, err = extbuf.ReadFrom(cipherbuf)
	if err != nil {
		return err
	}
	cipherpadding := make([]byte, (cipherbuf.Len()+3) & ^3)
	_, err = extbuf.Write(cipherpadding)
	if err != nil {
		return err

	}
	// FIXME Add additionalpadding as described in section 5.6 of nts draft?

	a.ExtHdr.Type = extAuthenticator
	a.ExtHdr.Length = 4 + uint16(extbuf.Len())
	err = a.ExtHdr.pack(buf)
	if err != nil {
		return err
	}

	_, err = buf.ReadFrom(extbuf)
	if err != nil {

		return err
	}
	//_, err = buf.Write(additionalpadding)
	//if err != nil {
	//	return err
	//}

	return nil
}

func (a *Authenticator) unpack(buf *bytes.Reader) error {
	if a.ExtHdr.Type != extAuthenticator {
		return fmt.Errorf("expected unpacked EF header")
	}

	// NonceLen, 2
	if err := binary.Read(buf, binary.BigEndian, &a.NonceLen); err != nil {
		return err
	}

	// CipherTextlen, 2
	if err := binary.Read(buf, binary.BigEndian, &a.CipherTextLen); err != nil {
		return err
	}

	// Nonce
	nonce := make([]byte, a.NonceLen)
	if err := binary.Read(buf, binary.BigEndian, &nonce); err != nil {
		return err
	}
	a.Nonce = nonce

	// Ciphertext
	ciphertext := make([]byte, a.CipherTextLen)
	if err := binary.Read(buf, binary.BigEndian, ciphertext); err != nil {
		return err
	}
	a.CipherText = ciphertext

	return nil
}
