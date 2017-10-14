package pss

import (
	crand "crypto/rand"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"io/ioutil"
	"os"
	"strings"
)

const (
	storeFileName = "pss.dat"
)

// temporary until default implementation of store exists in bzz. only used for testing
type stateStore struct {
	path    string
	key     *ecies.PrivateKey
	pubKeys []byte
}

func newStateStore(path string) (*stateStore, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	} else if !fi.IsDir() {
		return nil, fmt.Errorf("Not a directory")
	}
	fn := strings.Join([]string{path, storeFileName}, "/")
	_, err = os.OpenFile(fn, os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	return &stateStore{
		path: fn,
	}, nil
}

func (store *stateStore) Load(key string) (res []byte, err error) {
	switch key {
	case "pub":
		b, err := ioutil.ReadFile(store.path)
		if err != nil {
			return nil, err
		}
		res, err = store.key.Decrypt(crand.Reader, b, nil, nil)
		if err != nil {
			return nil, err
		}
	}
	return
}

func (store *stateStore) Save(key string, v []byte) error {
	return nil
}
