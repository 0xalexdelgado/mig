// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// Contributor: Julien Vehent jvehent@mozilla.com [:ulfr]
package sign

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"camlistore.org/pkg/misc/gpgagent"
	"camlistore.org/pkg/misc/pinentry"
	"code.google.com/p/go.crypto/openpgp"
)

// Sign signs a string with a key identified by a key fingerprint or an email address
func Sign(data, keyid string, secringFile io.Reader) (sig string, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("pgp.Sign(): %v", e)
		}
	}()
	keyring, err := openpgp.ReadKeyRing(secringFile)
	if err != nil {
		err = fmt.Errorf("Keyring access failed: '%v'", err)
		panic(err)
	}

	// find the entity in the keyring
	var signer *openpgp.Entity
	found := false
	for _, entity := range keyring {
		fingerprint := strings.ToUpper(hex.EncodeToString(entity.PrimaryKey.Fingerprint[:]))
		for _, ident := range entity.Identities {
			email := ident.UserId.Email
			if keyid == fingerprint || keyid == email {
				signer = entity
				found = true
				break
			}
		}
	}
	if !found {
		err = fmt.Errorf("Signer '%s' not found", keyid)
		panic(err)
	}

	// if private key is encrypted, attempt to decrypt it with the passphrase
	if signer.PrivateKey.Encrypted {
		// get private key passphrase
		signer, err = decryptEntity(signer)
		if err != nil {
			panic(err)
		}
	}

	// calculate signature
	out := bytes.NewBuffer(nil)
	message := bytes.NewBufferString(data)
	err = openpgp.ArmoredDetachSign(out, signer, message, nil)
	if err != nil {
		err = fmt.Errorf("Signature failed: '%v'", err)
		panic(err)
	}

	// convert the writer back to string
	sig, err = deArmor(out.String())
	if err != nil {
		err = fmt.Errorf("Error converting signature to string: '%v'", err)
		panic(err)
	}

	return
}

// deArmor takes a multi line armored GPG signature, and turns it back
// into a single line signature (thank you, camlistore folks)
func deArmor(sig string) (str string, err error) {
	index1 := strings.Index(sig, "\n\n")
	index2 := strings.Index(sig, "\n-----")
	if index1 == -1 || index2 == -1 {
		err = fmt.Errorf("Failed to parse signature from gpg.")
		return
	}
	inner := sig[index1+2 : index2]
	str = strings.Replace(inner, "\n", "", -1)
	return
}

// decryptEntity calls gnupg-agent and pinentry to obtain a passphrase and
// decrypt the private key of a given entity (thank you, camlistore folks)
func decryptEntity(s *openpgp.Entity) (ds *openpgp.Entity, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("pgp.decryptEntity(): %v", e)
		}
	}()
	ds = s
	// TODO: syscall.Mlock a region and keep pass phrase in it.
	pubk := &ds.PrivateKey.PublicKey
	desc := fmt.Sprintf("Need to unlock GPG key %s to use it for signing.",
		pubk.KeyIdShortString())

	conn, err := gpgagent.NewConn()
	switch err {
	case gpgagent.ErrNoAgent:
		fmt.Fprintf(os.Stderr, "Note: gpg-agent not found; resorting to on-demand password entry.\n")
	case nil:
		defer conn.Close()
		req := &gpgagent.PassphraseRequest{
			CacheKey: "mig:pgpsign:" + pubk.KeyIdShortString(),
			Prompt:   "Passphrase",
			Desc:     desc,
		}
		for tries := 0; tries < 2; tries++ {
			pass, err := conn.GetPassphrase(req)
			if err == nil {
				err = ds.PrivateKey.Decrypt([]byte(pass))
				if err == nil {
					return ds, err
				}
				req.Error = "Passphrase failed to decrypt: " + err.Error()
				conn.RemoveFromCache(req.CacheKey)
				continue
			}
			if err == gpgagent.ErrCancel {
				panic("failed to decrypt key; action canceled")
			}
		}
	default:
		panic(err)
	}

	pinReq := &pinentry.Request{Desc: desc, Prompt: "Passphrase"}
	for tries := 0; tries < 2; tries++ {
		pass, err := pinReq.GetPIN()
		if err == nil {
			err = ds.PrivateKey.Decrypt([]byte(pass))
			if err == nil {
				return ds, err
			}
			pinReq.Error = "Passphrase failed to decrypt: " + err.Error()
			continue
		}
		if err == pinentry.ErrCancel {
			panic("failed to decrypt key; action canceled")
		}
	}
	return ds, fmt.Errorf("decryptEntity(): failed to decrypt key %q", pubk.KeyIdShortString())
}
