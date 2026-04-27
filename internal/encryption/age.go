// Package encryption wraps a writer in age-encrypted output.
//
// We use age (https://age-encryption.org) because it has a stable on-disk
// format, no key-handling foot-guns, and supports both static recipients
// and SSH-style identities — the latter slots cleanly into KMS-managed
// signing flows (envelope encryption with a KMS-wrapped age identity).
package encryption

import (
	"errors"
	"io"
	"os"

	"filippo.io/age"
)

// EncryptOptions controls Encrypt.
type EncryptOptions struct {
	// RecipientStrings are age recipient strings (age1...) — at least one is required.
	// In KMS-managed mode the secret stays in the KMS; here we accept the
	// public recipient string only.
	RecipientStrings []string

	// IdentityFile (optional) — path to an age identity (private key) used
	// only when generating a self-encrypted recipient (rare; for tests).
	IdentityFile string
}

// NewEncryptingWriter returns a writer that encrypts everything written to it
// with age, then writes the ciphertext to dst. Close MUST be called.
func NewEncryptingWriter(dst io.Writer, opts EncryptOptions) (io.WriteCloser, error) {
	if len(opts.RecipientStrings) == 0 {
		return nil, errors.New("encryption: at least one recipient required")
	}
	recipients := make([]age.Recipient, 0, len(opts.RecipientStrings))
	for _, s := range opts.RecipientStrings {
		r, err := age.ParseX25519Recipient(s)
		if err != nil {
			return nil, err
		}
		recipients = append(recipients, r)
	}
	return age.Encrypt(dst, recipients...)
}

// LoadIdentity reads an age identity from disk. Used by `restore`.
func LoadIdentity(path string) (age.Identity, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	ids, err := age.ParseIdentities(f)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, errors.New("encryption: no identities in file")
	}
	return ids[0], nil
}
