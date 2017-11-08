package storage

import (
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/rpc"
)

type Resource struct {
	name string
	key  Key
}

type ResourceHandler struct {
	nod       *node.Node
	resources map[string]*Resource
	lock      *sync.Mutex
	dpa       *DPA
	hasher    SwarmHash
}

func NewResourceHandler(nod *node.Node, dpa *DPA) *ResourceHandler {
	return &ResourceHandler{
		nod:       nod,
		resources: make(map[string]*Resource),
		lock:      &sync.Mutex{},
		dpa:       dpa,
		hasher:    MakeHashFunc("SHA256")(),
	}
}

func (h *ResourceHandler) NewResource(name string) *Resource {
	h.resources[name] = &Resource{
		name: name,
	}
	return h.resources[name]
}

func (h *ResourceHandler) Set(hash Key, name string) error {
	if _, ok := h.resources[name]; !ok {
		return fmt.Errorf("unknown resource")
	}
	rpcclient, err := rpc.Dial(h.nod.IPCEndpoint())
	if err != nil {
		return err
	}
	var currentblock string
	err = rpcclient.Call(&currentblock, "eth_blockNumber")
	if err != nil {
		return err
	}
	h.lock.Lock()
	defer h.lock.Unlock()
	h.hasher.Reset()
	h.hasher.Write([]byte(currentblock))
	h.hasher.Write([]byte(name))
	key := h.hasher.Sum(nil)
	chunk := NewChunk(key, nil)
	chunk.SData = hash
	h.dpa.Put(chunk)
	h.resources[name].key = key
	return nil
}

func (h *ResourceHandler) Get(name string) Key {
	return h.resources[name].key
}
