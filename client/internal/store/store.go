// Package store keeps the PIN-encrypted private key on disk.
package store

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/secretbox"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/e2e"
)

// Argon2id parameters; vars so tests can lower the cost.
var (
	ArgonTime    uint32 = 3
	ArgonMemKiB  uint32 = 64 * 1024
	ArgonThreads uint8  = 1
	ArgonKeyLen  uint32 = 32
)

var ErrWrongPIN = errors.New("wrong PIN")

type KeyFile struct {
	Username  string `json:"username"`
	Server    string `json:"server"`
	PublicKey string `json:"publicKey"`
	Salt      string `json:"salt"`
	Nonce     string `json:"nonce"`
	Box       string `json:"box"`
}

// Dir returns the config directory (~/.umbrella, %APPDATA%\umbrella, or $UMBRELLA_HOME).
func Dir() (string, error) {
	if d := os.Getenv("UMBRELLA_HOME"); d != "" {
		return d, nil
	}
	if runtime.GOOS == "windows" {
		if a := os.Getenv("APPDATA"); a != "" {
			return filepath.Join(a, "umbrella"), nil
		}
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".umbrella"), nil
}

func derive(pin string, salt []byte) *[32]byte {
	var k [32]byte
	copy(k[:], argon2.IDKey([]byte(pin), salt, ArgonTime, ArgonMemKiB, ArgonThreads, ArgonKeyLen))
	return &k
}

func SaveKey(path, username, server, pin string, kp e2e.KeyPair) error {
	salt := make([]byte, 16)
	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return err
	}
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return err
	}
	sealed := secretbox.Seal(nil, kp.Private[:], &nonce, derive(pin, salt))
	kf := KeyFile{
		Username:  username,
		Server:    server,
		PublicKey: base64.StdEncoding.EncodeToString(kp.Public[:]),
		Salt:      base64.StdEncoding.EncodeToString(salt),
		Nonce:     base64.StdEncoding.EncodeToString(nonce[:]),
		Box:       base64.StdEncoding.EncodeToString(sealed),
	}
	data, err := json.MarshalIndent(kf, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	_ = os.Chmod(tmp, 0o600)
	return os.Rename(tmp, path)
}

func LoadKey(path, pin string) (KeyFile, e2e.KeyPair, error) {
	var kf KeyFile
	data, err := os.ReadFile(path)
	if err != nil {
		return kf, e2e.KeyPair{}, err
	}
	if err := json.Unmarshal(data, &kf); err != nil {
		return kf, e2e.KeyPair{}, err
	}
	salt, e1 := base64.StdEncoding.DecodeString(kf.Salt)
	nb, e2 := base64.StdEncoding.DecodeString(kf.Nonce)
	box, e3 := base64.StdEncoding.DecodeString(kf.Box)
	if e1 != nil || e2 != nil || e3 != nil || len(nb) != 24 {
		return kf, e2e.KeyPair{}, errors.New("corrupt key file")
	}
	var nonce [24]byte
	copy(nonce[:], nb)
	priv, ok := secretbox.Open(nil, box, &nonce, derive(pin, salt))
	if !ok || len(priv) != 32 {
		return kf, e2e.KeyPair{}, ErrWrongPIN
	}
	var kp e2e.KeyPair
	copy(kp.Private[:], priv)
	curve25519.ScalarBaseMult(&kp.Public, &kp.Private)
	return kf, kp, nil
}

// ExportKey copies the encrypted key file byte for byte.
func ExportKey(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600)
}
