// SPDX-License-Identifier: MPL-2.0
package connect

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Identity struct {
	Certificate string `json:"certificate"`
	Key         string `json:"key"`
}

func NewIdentity() (Identity, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return Identity{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return Identity{}, err
	}
	template := x509.Certificate{SerialNumber: serial, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(10, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return Identity{}, err
	}
	pk, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return Identity{}, err
	}
	return Identity{string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pk}))}, nil
}
func (i Identity) TLS() (tls.Certificate, error) {
	return tls.X509KeyPair([]byte(i.Certificate), []byte(i.Key))
}
func (i Identity) Pin() string {
	c, err := i.TLS()
	if err != nil {
		return ""
	}
	cert, err := x509.ParseCertificate(c.Certificate[0])
	if err != nil {
		return ""
	}
	return certificatePin(cert)
}
func certificatePin(c *x509.Certificate) string {
	h := sha256.Sum256(c.RawSubjectPublicKeyInfo)
	return hex.EncodeToString(h[:])
}
func randomID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func validID(s string) bool  { b, e := hex.DecodeString(s); return e == nil && len(b) == 32 }
func savePrivate(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".connect-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

type Invitation struct {
	Version int    `json:"v"`
	Broker  string `json:"broker"`
	Room    string `json:"room"`
	Pin     string `json:"pin"`
	Secret  string `json:"secret,omitempty"`
	STUN    string `json:"stun,omitempty"`
	Expires int64  `json:"expires,omitempty"`
}

func (i Invitation) URL() string {
	b, _ := json.Marshal(i)
	return "kurtz://pair#" + base64.RawURLEncoding.EncodeToString(b)
}
func ParseInvitation(raw string) (Invitation, error) {
	var i Invitation
	if len(raw) > 4096 || !strings.HasPrefix(raw, "kurtz://pair#") {
		return i, errors.New("Ungültiger Mutti-Kopplungslink.")
	}
	b, e := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(raw, "kurtz://pair#"))
	if e != nil {
		return i, e
	}
	if e = json.Unmarshal(b, &i); e != nil {
		return i, e
	}
	if e = i.Validate(); e != nil {
		return i, e
	}
	if !validID(i.Secret) || time.Now().Unix() >= i.Expires || i.Expires > time.Now().Add(11*time.Minute).Unix() {
		return i, errors.New("Der QR-Code ist ungültig oder abgelaufen. Bitte in Mutti einen neuen erstellen.")
	}
	return i, nil
}
func (i Invitation) Validate() error {
	if i.Version != 1 || !validID(i.Pin) || !validID(i.Room) {
		return errors.New("Ungültige Mutti-Verbindungsdaten.")
	}
	u, e := url.Parse(i.Broker)
	if e != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("Ungültiger Vermittlungsdienst.")
	}
	// HTTP is only allowed on private/local addresses. The media channel is independently pinned TLS.
	if u.Scheme != "https" && !(u.Scheme == "http" && privateHost(u.Hostname())) {
		return errors.New("Der öffentliche Vermittlungsdienst muss HTTPS verwenden.")
	}
	if i.STUN != "" && (!strings.HasPrefix(i.STUN, "stun:") || strings.ContainsAny(i.STUN, "@/?#\n\r")) {
		return errors.New("Nur STUN ist erlaubt. Relay ist deaktiviert.")
	}
	return nil
}
