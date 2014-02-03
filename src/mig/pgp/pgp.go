/* Mozilla InvestiGator PGP

Version: MPL 1.1/GPL 2.0/LGPL 2.1

The contents of this file are subject to the Mozilla Public License Version
1.1 (the "License"); you may not use this file except in compliance with
the License. You may obtain a copy of the License at
http://www.mozilla.org/MPL/

Software distributed under the License is distributed on an "AS IS" basis,
WITHOUT WARRANTY OF ANY KIND, either express or implied. See the License
for the specific language governing rights and limitations under the
License.

The Initial Developer of the Original Code is
Mozilla Corporation
Portions created by the Initial Developer are Copyright (C) 2014
the Initial Developer. All Rights Reserved.

Contributor(s):
Julien Vehent jvehent@mozilla.com [:ulfr]

Alternatively, the contents of this file may be used under the terms of
either the GNU General Public License Version 2 or later (the "GPL"), or
the GNU Lesser General Public License Version 2.1 or later (the "LGPL"),
in which case the provisions of the GPL or the LGPL are applicable instead
of those above. If you wish to allow use of your version of this file only
under the terms of either the GPL or the LGPL, and not to allow others to
use your version of this file under the terms of the MPL, indicate your
decision by deleting the provisions above and replace them with the notice
and other provisions required by the GPL or the LGPL. If you do not delete
the provisions above, a recipient may use your version of this file under
the terms of any one of the MPL, the GPL or the LGPL.
*/

package pgp

import (
	"bytes"
	"code.google.com/p/go.crypto/openpgp"
	"fmt"
	"io"
)

// TransformArmoredPubKeysToKeyring takes a list of public PGP key in armored form and transforms
// it into a keyring that can be used in other openpgp's functions
func ArmoredPubKeysToKeyring(pubkeys []string) (keyring io.Reader, keycount int, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("ArmoredePubKeysToKeyRing() -> %v", e)
		}
	}()
	var buf bytes.Buffer
	// iterate over the keys, and load them into an io.Reader keyring
	for _, key := range pubkeys {
		// Load PGP public key
		el, err := openpgp.ReadArmoredKeyRing(bytes.NewBufferString(key))
		if err != nil {
			panic(err)
		}
		keycount += 1
		if len(el) != 1 {
			err = fmt.Errorf("Public GPG Key contains %d entities, wanted 1\n", len(el))
			panic(err)
		}
		// serialize entities into io.Reader
		err = el[0].Serialize(&buf)
		if err != nil {
			panic(err)
		}
	}
	keyring = bytes.NewReader(buf.Bytes())
	return
}
