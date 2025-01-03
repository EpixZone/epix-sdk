//go:build rocksdb
// +build rocksdb

package storage_test

import (
	"bytes"
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	corestore "cosmossdk.io/core/store"
	coretesting "cosmossdk.io/core/testing"
	"cosmossdk.io/store/v2"
	"cosmossdk.io/store/v2/storage"
	"cosmossdk.io/store/v2/storage/pebbledb"
	"cosmossdk.io/store/v2/storage/rocksdb"
	"cosmossdk.io/store/v2/storage/sqlite"
)

var storeKey1 = []byte("store1")

var (
	backends = map[string]func(dataDir string) (store.VersionedWriter, error){
		"rocksdb_versiondb_opts": func(dataDir string) (store.VersionedWriter, error) {
			db, err := rocksdb.New(dataDir)
			return storage.NewStorageStore(db, coretesting.NewNopLogger()), err
		},
		"pebbledb_default_opts": func(dataDir string) (store.VersionedWriter, error) {
			db, err := pebbledb.New(dataDir)
			if err == nil && db != nil {
				db.SetSync(false)
			}

			return storage.NewStorageStore(db, coretesting.NewNopLogger()), err
		},
		"btree_sqlite": func(dataDir string) (store.VersionedWriter, error) {
			db, err := sqlite.New(dataDir)
			return storage.NewStorageStore(db, coretesting.NewNopLogger()), err
		},
	}
	rng = rand.New(rand.NewSource(567320))
)

func BenchmarkGet(b *testing.B) {
	numKeyVals := 1_000_000
	keys := make([][]byte, numKeyVals)
	vals := make([][]byte, numKeyVals)
	for i := 0; i < numKeyVals; i++ {
		key := make([]byte, 128)
		val := make([]byte, 128)

		_, err := rng.Read(key)
		require.NoError(b, err)
		_, err = rng.Read(val)
		require.NoError(b, err)

		keys[i] = key
		vals[i] = val
	}

	for ty, fn := range backends {
		db, err := fn(b.TempDir())
		require.NoError(b, err)
		defer func() {
			_ = db.Close()
		}()

		cs := corestore.NewChangesetWithPairs(1, map[string]corestore.KVPairs{string(storeKey1): {}})
		for i := 0; i < numKeyVals; i++ {
			cs.AddKVPair(storeKey1, corestore.KVPair{Key: keys[i], Value: vals[i]})
		}

		require.NoError(b, db.ApplyChangeset(cs))

		b.Run(fmt.Sprintf("backend_%s", ty), func(b *testing.B) {
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				key := keys[rng.Intn(len(keys))]

				b.StartTimer()
				_, err = db.Get(storeKey1, 1, key)
				require.NoError(b, err)
			}
		})
	}
}

func BenchmarkApplyChangeset(b *testing.B) {
	for ty, fn := range backends {
		db, err := fn(b.TempDir())
		require.NoError(b, err)
		defer func() {
			_ = db.Close()
		}()

		b.Run(fmt.Sprintf("backend_%s", ty), func(b *testing.B) {
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()

				ver := uint64(b.N + 1)
				cs := corestore.NewChangesetWithPairs(ver, map[string]corestore.KVPairs{string(storeKey1): {}})
				for j := 0; j < 1000; j++ {
					key := make([]byte, 128)
					val := make([]byte, 128)

					_, err = rng.Read(key)
					require.NoError(b, err)
					_, err = rng.Read(val)
					require.NoError(b, err)

					cs.AddKVPair(storeKey1, corestore.KVPair{Key: key, Value: val})
				}

				b.StartTimer()
				require.NoError(b, db.ApplyChangeset(cs))
			}
		})
	}
}

func BenchmarkIterate(b *testing.B) {
	numKeyVals := 1_000_000
	keys := make([][]byte, numKeyVals)
	vals := make([][]byte, numKeyVals)
	for i := 0; i < numKeyVals; i++ {
		key := make([]byte, 128)
		val := make([]byte, 128)

		_, err := rng.Read(key)
		require.NoError(b, err)
		_, err = rng.Read(val)
		require.NoError(b, err)

		keys[i] = key
		vals[i] = val

	}

	for ty, fn := range backends {
		db, err := fn(b.TempDir())
		require.NoError(b, err)
		defer func() {
			_ = db.Close()
		}()

		b.StopTimer()

		cs := corestore.NewChangesetWithPairs(1, map[string]corestore.KVPairs{string(storeKey1): {}})
		for i := 0; i < numKeyVals; i++ {
			cs.AddKVPair(storeKey1, corestore.KVPair{Key: keys[i], Value: vals[i]})
		}

		require.NoError(b, db.ApplyChangeset(cs))

		sort.Slice(keys, func(i, j int) bool {
			return bytes.Compare(keys[i], keys[j]) < 0
		})

		b.Run(fmt.Sprintf("backend_%s", ty), func(b *testing.B) {
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()

				itr, err := db.Iterator(storeKey1, 1, keys[0], nil)
				require.NoError(b, err)

				b.StartTimer()

				for ; itr.Valid(); itr.Next() {
					_ = itr.Key()
					_ = itr.Value()
				}

				require.NoError(b, itr.Error())
			}
		})
	}
}
