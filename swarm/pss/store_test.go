package pss

import (
	"crypto/ecdsa"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/swarm/network"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv5"
	"testing"
)

func init() {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StderrHandler))
}

func TestStateCorePub(t *testing.T) {
	var topics []whisper.TopicType
	var keys []*ecdsa.PrivateKey
	var addrs []PssAddress
	for i := 0; i < 4; i++ {
		key, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, key)
		addr := make(PssAddress, 32)
		copy(addr[:], network.RandomAddr().Over())
		addrs = append(addrs, addr)
		topics = append(topics, BytesToTopic([]byte(addr)))
	}
	psp := NewPssParams(keys[0])
	ps := newTestPss(keys[0], psp)

	for i, k := range keys {
		ps.SetPeerPublicKey(&k.PublicKey, topics[i], &addrs[i])
	}
}
