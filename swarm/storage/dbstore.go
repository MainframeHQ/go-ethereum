// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// disk storage layer for the package bzz
// DbStore implements the ChunkStore interface and is used by the DPA as
// persistent storage of chunks
// it implements purging based on access count allowing for external control of
// max capacity

package storage

import (
	//"archive/tar"
	"bytes"
	"encoding/binary"
	//"encoding/hex"
	"fmt"
	//"io"
	//"io/ioutil"
	"sync"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
)

const (
	defaultDbCapacity = 5000000
	defaultRadius     = 0 // not yet used

	gcArraySize      = 10000
	gcArrayFreeRatio = 0.1

	dbVersion = 3
	dbTypes   = 2
)

// unallocated for new types: [3,6]
const (
	DB_GC      = 0
	DB_VERSION = 0xff

	DB_DATA     = 2
	DB_RESOURCE = 3
)

// cannot be 1, GC key is upper boundary
func isValidEntrytype(entrytype uint8) bool {
	if entrytype == 1 && entrytype > 6 {
		return false
	}
	return true
}

func getDataPrefix(entrytype uint8) []byte {
	if !isValidEntrytype(entrytype) {
		return nil
	}
	return []byte{1 << entrytype}
}

func getIndexPrefix(entrytype uint8) []byte {
	if !isValidEntrytype(entrytype) {
		return nil
	}
	return []byte{(1 << entrytype) + 1}
}

func getEntryPrefix(entrytype uint8) []byte {
	if !isValidEntrytype(entrytype) {
		return nil
	}
	return []byte{(1 << entrytype) + 2}
}

func getAccessPrefix(entrytype uint8) []byte {
	if !isValidEntrytype(entrytype) {
		return nil
	}
	return []byte{(1 << entrytype) + 3}
}

type gcItem struct {
	idx    uint64
	value  uint64
	idxKey []byte
}

type dbCursor struct {
	entry  uint64
	access uint64
	gc     []byte
}

type DbStore struct {
	db *LDBDatabase

	// this should be stored in db, accessed transactionally
	dataIdx, capacity, version uint64

	gcStartPos []byte
	gcArray    []*gcItem

	// counters for each data type
	cursor []*dbCursor

	hashfunc SwarmHasher

	lock sync.Mutex
}

func NewDbStore(path string, hash SwarmHasher, capacity uint64, radius int) (s *DbStore, err error) {
	s = new(DbStore)

	s.hashfunc = hash

	s.db, err = NewLDBDatabase(path)
	if err != nil {
		return
	}

	v, err := s.db.Get([]byte{DB_VERSION})
	if err != nil {
		return nil, fmt.Errorf("obsolete db < v3, please upgrade")
	}
	s.version = BytesToU64(v)
	if s.version != dbVersion {
		return nil, fmt.Errorf("obsolete db v%d, please upgrade", s.version)
	}

	s.setCapacity(capacity)

	s.gcStartPos = make([]byte, 1)
	s.gcStartPos[0] = 0
	s.gcArray = make([]*gcItem, gcArraySize)

	gc, _ := s.db.Get([]byte{DB_GC})
	for i, typ := range []uint8{DB_DATA, DB_RESOURCE} {
		cursor := &dbCursor{}
		entry, _ := s.db.Get(getEntryPrefix(typ))
		if len(entry) > 0 {
			cursor.entry = BytesToU64(entry)
			if cursor.entry > 0 {
				cursor.entry++
			}
		}
		access, _ := s.db.Get(getAccessPrefix(typ))
		if len(access) > 0 {
			cursor.access = BytesToU64(access)
			if cursor.access > 0 {
				cursor.access++
			}
		}
		if len(gc) > 0 {
			cursor.gc = gc[i : i+8]
		}
		s.cursor = append(s.cursor, cursor)
	}
	return
}

type dpaDBIndex struct {
	Idx    uint64
	Access uint64
}

func BytesToU64(data []byte) uint64 {
	if len(data) < 8 {
		return 0
	}
	return binary.LittleEndian.Uint64(data)
}

func U64ToBytes(val uint64) []byte {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, val)
	return data
}

func getIndexGCValue(index *dpaDBIndex) uint64 {
	return index.Access
}

func getDataKey(idx uint64, entrytype uint8) []byte {
	pf := getDataPrefix(entrytype)
	if pf == nil {
		return nil
	}
	key := make([]byte, 9)
	key[0] = pf[0]
	binary.BigEndian.PutUint64(key[1:9], idx)
	return key
}

func getIndexKey(hash Key, entrytype uint8) []byte {
	pf := getIndexPrefix(entrytype)
	if pf == nil {
		return pf
	}
	HashSize := len(hash)
	key := make([]byte, HashSize+1)
	key[0] = pf[0]
	copy(key[1:], hash[:])
	return key

}

func (s *DbStore) getAccessCount(entrytype uint8) uint64 {
	return s.cursor[entrytype-2].access
}

func (s *DbStore) getEntryCount(entrytype uint8) uint64 {
	return s.cursor[entrytype-2].entry
}

func encodeIndex(index *dpaDBIndex) []byte {
	data, _ := rlp.EncodeToBytes(index)
	return data
}

func encodeData(chunk *Chunk) []byte {
	return chunk.SData
}

func decodeIndex(data []byte, index *dpaDBIndex) {
	dec := rlp.NewStream(bytes.NewReader(data), 0)
	dec.Decode(index)
}

func decodeData(data []byte, chunk *Chunk) {
	chunk.SData = data
	chunk.Size = int64(binary.LittleEndian.Uint64(data[0:8]))
}

func gcListPartition(list []*gcItem, left int, right int, pivotIndex int) int {
	pivotValue := list[pivotIndex].value
	dd := list[pivotIndex]
	list[pivotIndex] = list[right]
	list[right] = dd
	storeIndex := left
	for i := left; i < right; i++ {
		if list[i].value < pivotValue {
			dd = list[storeIndex]
			list[storeIndex] = list[i]
			list[i] = dd
			storeIndex++
		}
	}
	dd = list[storeIndex]
	list[storeIndex] = list[right]
	list[right] = dd
	return storeIndex
}

func gcListSelect(list []*gcItem, left int, right int, n int) int {
	if left == right {
		return left
	}
	pivotIndex := (left + right) / 2
	pivotIndex = gcListPartition(list, left, right, pivotIndex)
	if n == pivotIndex {
		return n
	} else {
		if n < pivotIndex {
			return gcListSelect(list, left, pivotIndex-1, n)
		} else {
			return gcListSelect(list, pivotIndex+1, right, n)
		}
	}
}

// TODO: only collect garbage on total capacity exceeded (all data types)
func (s *DbStore) collectGarbage(ratio float32) {
	newgccount := make([]byte, len(s.cursor)*8)
	for i, cursor := range s.cursor {
		it := s.db.NewIterator()
		it.Seek(cursor.gc)
		if it.Valid() {
			cursor.gc = it.Key()
		} else {
			cursor.gc = nil
		}
		gcnt := 0

		for (gcnt < gcArraySize) && (uint64(gcnt) < cursor.entry) {

			if (cursor.gc == nil) || (cursor.gc[0] != 0) {
				it.Seek(s.gcStartPos)
				if it.Valid() {
					cursor.gc = it.Key()
				} else {
					cursor.gc = nil
				}
			}

			if (cursor.gc == nil) || (cursor.gc[0] != 0) {
				break
			}

			gci := new(gcItem)
			gci.idxKey = cursor.gc
			var index dpaDBIndex
			decodeIndex(it.Value(), &index)
			gci.idx = index.Idx
			// the smaller, the more likely to be gc'd
			gci.value = getIndexGCValue(&index)
			s.gcArray[gcnt] = gci
			gcnt++
			it.Next()
			if it.Valid() {
				cursor.gc = it.Key()
			} else {
				cursor.gc = nil
			}
		}
		it.Release()

		cutidx := gcListSelect(s.gcArray, 0, gcnt-1, int(float32(gcnt)*ratio))
		cutval := s.gcArray[cutidx].value

		// fmt.Print(gcnt, " ", s.entryCnt, " ")

		// actual gc
		for i := 0; i < gcnt; i++ {
			if s.gcArray[i].value <= cutval {
				s.delete(s.gcArray[i].idx, s.gcArray[i].idxKey, uint8(i+2))
			}
		}
		copy(newgccount[i*8:(i*8)+8], cursor.gc)
		// fmt.Println(s.entryCnt)

	}

	s.db.Put([]byte{DB_GC}, newgccount)
}

//// Export writes all chunks from the store to a tar archive, returning the
//// number of chunks written.
//func (s *DbStore) Export(out io.Writer) (int64, error) {
//	tw := tar.NewWriter(out)
//	defer tw.Close()
//
//	it := s.db.NewIterator()
//	defer it.Release()
//	var count int64
//	for ok := it.Seek([]byte{kpIndex}); ok; ok = it.Next() {
//		key := it.Key()
//		if (key == nil) || (key[0] != kpIndex) {
//			break
//		}
//
//		var index dpaDBIndex
//		decodeIndex(it.Value(), &index)
//
//		data, err := s.db.Get(getDataKey(index.Idx))
//		if err != nil {
//			log.Warn(fmt.Sprintf("Chunk %x found but could not be accessed: %v", key[:], err))
//			continue
//		}
//
//		hdr := &tar.Header{
//			Name: hex.EncodeToString(key[1:]),
//			Mode: 0644,
//			Size: int64(len(data)),
//		}
//		if err := tw.WriteHeader(hdr); err != nil {
//			return count, err
//		}
//		if _, err := tw.Write(data); err != nil {
//			return count, err
//		}
//		count++
//	}
//
//	return count, nil
//}
//
//// Import reads chunks into the store from a tar archive, returning the number
//// of chunks read.
//func (s *DbStore) Import(in io.Reader) (int64, error) {
//	tr := tar.NewReader(in)
//
//	var count int64
//	for {
//		hdr, err := tr.Next()
//		if err == io.EOF {
//			break
//		} else if err != nil {
//			return count, err
//		}
//
//		if len(hdr.Name) != 64 {
//			log.Warn("ignoring non-chunk file", "name", hdr.Name)
//			continue
//		}
//
//		key, err := hex.DecodeString(hdr.Name)
//		if err != nil {
//			log.Warn("ignoring invalid chunk file", "name", hdr.Name, "err", err)
//			continue
//		}
//
//		data, err := ioutil.ReadAll(tr)
//		if err != nil {
//			return count, err
//		}
//
//		s.Put(&Chunk{Key: key, SData: data})
//		count++
//	}
//
//	return count, nil
//}

//func (s *DbStore) Cleanup() {
//	//Iterates over the database and checks that there are no faulty chunks
//	it := s.db.NewIterator()
//	startPosition := []byte{kpIndex}
//	it.Seek(startPosition)
//	var key []byte
//	var errorsFound, total int
//	for it.Valid() {
//		key = it.Key()
//		if (key == nil) || (key[0] != kpIndex) {
//			break
//		}
//		total++
//		var index dpaDBIndex
//		decodeIndex(it.Value(), &index)
//
//		data, err := s.db.Get(getDataKey(index.Idx))
//		if err != nil {
//			log.Warn(fmt.Sprintf("Chunk %x found but could not be accessed: %v", key[:], err))
//			s.delete(index.Idx, getIndexKey(key[1:]))
//			errorsFound++
//		} else {
//			hasher := s.hashfunc()
//			hasher.Write(data)
//			hash := hasher.Sum(nil)
//			if !bytes.Equal(hash, key[1:]) {
//				log.Warn(fmt.Sprintf("Found invalid chunk. Hash mismatch. hash=%x, key=%x", hash, key[:]))
//				s.delete(index.Idx, getIndexKey(key[1:]))
//				errorsFound++
//			}
//		}
//		it.Next()
//	}
//	it.Release()
//	log.Warn(fmt.Sprintf("Found %v errors out of %v entries", errorsFound, total))
//}

func (s *DbStore) delete(idx uint64, idxHash []byte, entrytype uint8) {
	ikey := getIndexKey(idxHash, entrytype)
	if ikey == nil {
		return
	}
	dkey := getDataKey(idx, entrytype)
	if dkey == nil {
		return
	}
	batch := new(leveldb.Batch)
	batch.Delete(ikey)
	batch.Delete(dkey)
	s.db.Write(batch)
}

//func (s *DbStore) Counter() uint64 {
//	s.lock.Lock()
//	defer s.lock.Unlock()
//	return s.dataIdx
//}

func (s *DbStore) Put(chunk *Chunk, entrytype uint8) {
	s.lock.Lock()
	defer s.lock.Unlock()

	ikey := getIndexKey(chunk.Key, entrytype)

	var index dpaDBIndex

	if s.tryAccessIdx(ikey, &index, entrytype) {
		if chunk.dbStored != nil {
			close(chunk.dbStored)
		}
		log.Trace(fmt.Sprintf("Storing to DB: chunk already exists, only update access"))
		return // already exists, only update access
	}

	data := encodeData(chunk)
	//data := ethutil.Encode([]interface{}{entry})

	if s.cursor[entrytype-2].entry >= s.capacity {
		s.collectGarbage(gcArrayFreeRatio)
	}

	batch := new(leveldb.Batch)

	batch.Put(getDataKey(s.dataIdx, entrytype), data)

	index.Idx = s.dataIdx

	idata := encodeIndex(&index)
	batch.Put(ikey, idata)

	batch.Put(getEntryPrefix(entrytype), U64ToBytes(s.cursor[entrytype-2].entry))
	s.cursor[entrytype-2].entry++

	batch.Put(getAccessPrefix(entrytype), U64ToBytes(s.cursor[entrytype-2].access))
	s.cursor[entrytype-2].access++

	s.db.Write(batch)

	if chunk.dbStored != nil {
		close(chunk.dbStored)
	}

	log.Trace(fmt.Sprintf("DbStore.Put: %v. db storage counter: %v ", chunk.Key.Log(), s.dataIdx))
}

// try to find index; if found, update access cnt and return true
//func (s *DbStore) tryAccessIdx(ikey []byte, index *dpaDBIndex, entrytype uint8) bool {
func (s *DbStore) tryAccessIdx(ikey []byte, index *dpaDBIndex, entrytype uint8) bool {
	idata, err := s.db.Get(ikey)
	if err != nil {
		return false
	}
	decodeIndex(idata, index)

	batch := new(leveldb.Batch)

	batch.Put(getAccessPrefix(entrytype), U64ToBytes(s.cursor[entrytype-2].access))
	s.cursor[entrytype-2].access++
	idata = encodeIndex(index)
	batch.Put(ikey, idata)

	s.db.Write(batch)

	return true
}

func (s *DbStore) Get(key Key, entrytype uint8) (chunk *Chunk, err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	var index dpaDBIndex

	if s.tryAccessIdx(getIndexKey(key, entrytype), &index, entrytype) {
		var data []byte
		data, err = s.db.Get(getDataKey(index.Idx, entrytype))
		if err != nil {
			log.Trace(fmt.Sprintf("DBStore: Chunk %v found but could not be accessed: %v", key.Log(), err))
			s.delete(index.Idx, getIndexKey(key, entrytype), entrytype)
			return
		}

		if entrytype == DB_DATA {
			hasher := s.hashfunc()
			hasher.Write(data)
			hash := hasher.Sum(nil)
			if !bytes.Equal(hash, key) {
				s.delete(index.Idx, getIndexKey(key, entrytype), entrytype)
				log.Warn("Invalid Chunk in Database. Please repair with command: 'swarm cleandb'")
			}
		}

		chunk = &Chunk{
			Key: key,
		}
		decodeData(data, chunk)
	} else {
		err = notFound
	}

	return

}

func (s *DbStore) updateAccessCnt(key Key, entrytype uint8) {

	s.lock.Lock()
	defer s.lock.Unlock()

	var index dpaDBIndex

	s.tryAccessIdx(getIndexKey(key, entrytype), &index, entrytype) // result_chn == nil, only update access cnt

}

func (s *DbStore) setCapacity(c uint64) {

	s.lock.Lock()
	defer s.lock.Unlock()

	s.capacity = c

	var totalentries uint64
	for i, cursor := range s.cursor {
		totalentries += cursor.entry
	}
	if totalentries > c {
		ratio := float32(1.01) - float32(c)/float32(totalentries)
		if ratio < gcArrayFreeRatio {
			ratio = gcArrayFreeRatio
		}
		if ratio > 1 {
			ratio = 1
		}
		for totalentries > c {
			s.collectGarbage(ratio)
			break
		}
	}
}

func (s *DbStore) Close() {
	s.db.Close()
}

//  describes a section of the DbStore representing the unsynced
// domain relevant to a peer
// Start - Stop designate a continuous area Keys in an address space
// typically the addresses closer to us than to the peer but not closer
// another closer peer in between
// From - To designates a time interval typically from the last disconnect
// till the latest connection (real time traffic is relayed)
type DbSyncState struct {
	Start, Stop Key
	First, Last uint64
}

// implements the syncer iterator interface
// iterates by storage index (~ time of storage = first entry to db)
type dbSyncIterator struct {
	it iterator.Iterator
	DbSyncState
}

// initialises a sync iterator from a syncToken (passed in with the handshake)
func (self *DbStore) NewSyncIterator(state DbSyncState, entrytype uint8) (si *dbSyncIterator, err error) {
	if state.First > state.Last {
		return nil, fmt.Errorf("no entries found")
	}
	si = &dbSyncIterator{
		it:          self.db.NewIterator(),
		DbSyncState: state,
	}
	si.it.Seek(getIndexKey(state.Start, entrytype))
	return si, nil
}

// walk the area from Start to Stop and returns items within time interval
// First to Last
func (self *dbSyncIterator) Next() (key Key) {
	for self.it.Valid() {
		dbkey := self.it.Key()
		if dbkey[0] != 0 {
			break
		}
		key = Key(make([]byte, len(dbkey)-1))
		copy(key[:], dbkey[1:])
		if bytes.Compare(key[:], self.Start) <= 0 {
			self.it.Next()
			continue
		}
		if bytes.Compare(key[:], self.Stop) > 0 {
			break
		}
		var index dpaDBIndex
		decodeIndex(self.it.Value(), &index)
		self.it.Next()
		if (index.Idx >= self.First) && (index.Idx < self.Last) {
			return
		}
	}
	self.it.Release()
	return nil
}
