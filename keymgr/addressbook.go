// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package keymgr

import (
	"github.com/monetas/bmutil/identity"
)

type AddressBook interface {
	LookupPublicIdentity(string) (*identity.Public, error)
	//AddPublicIdentity(string, *identity.Public)
	LookupPrivateIdentity(string) (*identity.Private, error)
}

/*type addressBook struct {
	managers []*Manager
	addrs    map[string]*identity.Public
}

func (book *addressBook) LookupPublicIdentity(str *string) *identity.Public {
	identity, ok := book.addrs[*str]
	if ok {
		reutrn identity, nil
	}

	private, err := book.LookupPrivateIdentity(str)
	if err != nil {
		return nil, err
	}

	return private.ToPublic(), nil
}

func (book *addressBook) AddPublicIdentity(str *string, public *identity.Public) {
	book.addrs[*str] = public
}

func (book *addressBook) LookupPrivateIdentity(str *string) *identity.Private {
	for _, manager := range book.managers {
		identity, _ := manager.LookupByAddress(*str)
		if identity != nil {
			return identity.ToPublic(), nil
		}
	}

	return ErrNonexistentIdentity
}*/
