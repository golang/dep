// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/boltdb/bolt"
	"github.com/golang/dep/internal/gps/pkgtree"
	"github.com/pkg/errors"
)

// boltCache manages a bolt.DB cache and provides singleSourceCaches.
type boltCache struct {
	db     *bolt.DB
	epoch  int64       // getters will not return values older than this unix timestamp
	logger *log.Logger // info logging
}

// newBoltCache returns a new boltCache backed by a BoltDB file under the cache directory.
func newBoltCache(cd string, epoch int64, logger *log.Logger) (*boltCache, error) {
	path := sourceCachePath(cd, "bolt") + ".db"
	dir := filepath.Dir(path)
	if fi, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, os.ModeDir|os.ModePerm); err != nil {
			return nil, errors.Wrapf(err, "failed to create source cache directory: %s", dir)
		}
	} else if err != nil {
		return nil, errors.Wrapf(err, "failed to check source cache directory: ", dir)
	} else if !fi.IsDir() {
		return nil, errors.Wrapf(err, "source cache path is not directory: %s", dir)
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open BoltDB cache file %q", path)
	}
	return &boltCache{
		db:     db,
		epoch:  epoch,
		logger: logger,
	}, nil
}

// newSingleSourceCache returns a new singleSourceCache for pi.
func (c *boltCache) newSingleSourceCache(pi ProjectIdentifier) singleSourceCache {
	return &singleSourceCacheBolt{
		boltCache:  c,
		pi:         pi,
		sourceName: []byte(pi.normalizedSource()),
	}
}

// close releases all cache resources.
func (c *boltCache) close() error {
	return errors.Wrapf(c.db.Close(), "error closing Bolt database %q", c.db.String())
}

// singleSourceCacheBolt implements a singleSourceCache backed by a persistent BoltDB file.
// Version mappings are timestamped, and the `epoch` field limits the age of returned values.
// Database access methods are safe for concurrent use.
//
// Implementation:
//
// Each source has a top-level bucket containing sub-buckets for (1) versions and (2) revisions.
//
// 1) Versions buckets hold version keys with revision values:
//
//	Bucket: "versions:<timestamp>"
//	Keys: "branch:<branch>", "defaultBranch:<branch>", "ver:<version>"
//	Values: "<revision>"
//
// 2) Revision buckets hold (a) manifest and lock data for various ProjectAnalyzers,
// (b) package trees, and (c) version lists.
//
//	Bucket: "rev:<revision>"
//
// a) Manifest and Lock info are stored in buckets derived from ProjectAnalyzer.Info:
//
//	Sub-Bucket: "info:<name>.<version>:manifest", "info:<name>.<version>:lock"
//	Keys/Values: Manifest or Lock fields
//
// b) Package tree buckets contain package import path keys and package-or-error buckets:
//
//	Sub-Bucket: "ptree"
//	Sub-Bucket: "<import_path>"
//	Key/Values: PackageOrErr fields
//
// c) Revision-versions buckets contain lists of version values:
//
//	Sub-Bucket: "versions:<timestamp>"
//	Keys: "<sequence_number>"
//	Values: "branch:<branch>", "defaultBranch:<branch>", "ver:<version>"
type singleSourceCacheBolt struct {
	*boltCache
	pi         ProjectIdentifier
	sourceName []byte
}

func (s *singleSourceCacheBolt) setManifestAndLock(rev Revision, ai ProjectAnalyzerInfo, m Manifest, l Lock) {
	err := s.updateRevBucket(rev, func(b *bolt.Bucket) error {
		mName, lName := cacheInfoNames(ai)
		if b.Bucket(mName) != nil {
			if err := b.DeleteBucket(mName); err != nil {
				return err
			}
		}
		if b.Bucket(lName) != nil {
			if err := b.DeleteBucket(lName); err != nil {
				return err
			}
		}

		// Manifest
		mb, err := b.CreateBucket(mName)
		if err != nil {
			return err
		}
		if err := cachePutManifest(mb, m); err != nil {
			return errors.Wrap(err, "failed to put manifest")
		}
		if l == nil {
			return nil
		}

		// Lock
		lb, err := b.CreateBucket(lName)
		if err != nil {
			return err
		}
		return errors.Wrap(cachePutLock(lb, l), "failed to put lock")
	})
	if err != nil {
		s.logger.Println(errors.Wrapf(err, "failed to cache manifest/lock for revision %q, analyzer: %v", rev, ai))
	}
}

// cache info bucket prefixes and suffixes.
var (
	cacheInfo     = []byte("info:")
	cacheManifest = []byte(":manifest")
	cacheLock     = []byte(":lock")
)

// cacheInfoNames returns the manifest and lock bucket names for ai.
func cacheInfoNames(ai ProjectAnalyzerInfo) (manifest, lock []byte) {
	info := ai.String()
	l := len(cacheInfo) + len(info)
	manifest = make([]byte, l+len(cacheManifest))
	lock = make([]byte, l+len(cacheLock))

	copy(manifest, cacheInfo)
	copy(manifest[len(cacheInfo):], info)
	copy(manifest[l:], cacheManifest)

	copy(lock, cacheInfo)
	copy(lock[len(cacheInfo):], info)
	copy(lock[l:], cacheLock)

	return
}

func (s *singleSourceCacheBolt) getManifestAndLock(rev Revision, ai ProjectAnalyzerInfo) (m Manifest, l Lock, ok bool) {
	err := s.viewRevBucket(rev, func(b *bolt.Bucket) error {
		mName, lName := cacheInfoNames(ai)

		// Manifest
		mb := b.Bucket(mName)
		if mb == nil {
			return nil
		}
		var err error
		m, err = cacheGetManifest(mb)
		if err != nil {
			return errors.Wrap(err, "failed to get manifest")
		}

		// Lock
		lb := b.Bucket(lName)
		if lb == nil {
			ok = true
			return nil
		}
		l, err = cacheGetLock(lb)
		if err != nil {
			return errors.Wrap(err, "failed to get lock")
		}

		ok = true
		return nil
	})
	if err != nil {
		s.logger.Println(errors.Wrapf(err, "failed to get cached manifest/lock for revision %q, analyzer: %v", rev, ai))
	}
	return
}

var cachePTree = []byte("ptree")

func (s *singleSourceCacheBolt) setPackageTree(rev Revision, ptree pkgtree.PackageTree) {
	err := s.updateRevBucket(rev, func(b *bolt.Bucket) error {
		if b.Bucket(cachePTree) != nil {
			if err := b.DeleteBucket(cachePTree); err != nil {
				return err
			}
		}
		ptrees, err := b.CreateBucket(cachePTree)
		if err != nil {
			return err
		}

		for ip, poe := range ptree.Packages {
			pb, err := ptrees.CreateBucket([]byte(ip))
			if err != nil {
				return err
			}

			if err := cachePutPackageOrErr(pb, poe); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		s.logger.Println(errors.Wrapf(err, "failed to cache package tree for revision %q", rev))
	}
}

func (s *singleSourceCacheBolt) getPackageTree(rev Revision) (ptree pkgtree.PackageTree, ok bool) {
	err := s.viewRevBucket(rev, func(b *bolt.Bucket) error {
		ptrees := b.Bucket(cachePTree)
		if ptrees == nil {
			return nil
		}

		pkgs := make(map[string]pkgtree.PackageOrErr)
		err := ptrees.ForEach(func(ip, _ []byte) error {
			poe := cacheGetPackageOrErr(ptrees.Bucket(ip))
			if poe.Err == nil {
				poe.P.ImportPath = string(ip)
			}
			pkgs[string(ip)] = poe
			return nil
		})
		if err != nil {
			return err
		}
		ptree.ImportRoot = string(s.pi.ProjectRoot)
		ptree.Packages = pkgs
		ok = true
		return nil
	})
	if err != nil {
		s.logger.Println(errors.Wrapf(err, "failed to get cached package tree for revision %q", rev))
	}
	return
}

func (s *singleSourceCacheBolt) markRevisionExists(rev Revision) {
	err := s.updateRevBucket(rev, func(versions *bolt.Bucket) error {
		return nil
	})
	if err != nil {
		s.logger.Println(errors.Wrapf(err, "failed to mark revision %q in cache", rev))
	}
}

func (s *singleSourceCacheBolt) setVersionMap(pvs []PairedVersion) {
	err := s.updateSourceBucket(func(src *bolt.Bucket) error {
		if err := cachePrefixDelete(src, "versions:"); err != nil {
			return err
		}
		vk := cacheTimestampedKey("versions:", time.Now())
		versions, err := src.CreateBucket(vk)
		if err != nil {
			return err
		}

		c := src.Cursor()
		for k, _ := c.Seek(cacheRev); bytes.HasPrefix(k, cacheRev); k, _ = c.Next() {
			rb := src.Bucket(k)
			if err := cachePrefixDelete(rb, "versions:"); err != nil {
				return err
			}
		}

		for _, pv := range pvs {
			uv, rev := pv.Unpair(), pv.Revision()
			uvB, err := cacheEncodeUnpairedVersion(uv)
			if err != nil {
				return errors.Wrapf(err, "failed to encode unpaired version: %v", uv)
			}

			if err := versions.Put(uvB, []byte(rev)); err != nil {
				return errors.Wrap(err, "failed to put version->revision")
			}

			b, err := src.CreateBucketIfNotExists(cacheRevisionName(rev))
			if err != nil {
				return errors.Wrapf(err, "failed to create bucket for revision: %s", rev)
			}
			if err := cachePrefixDelete(b, "versions:"); err != nil {
				return err
			}
			versions, err := b.CreateBucket(vk)
			if err != nil {
				return errors.Wrapf(err, "failed to create bucket for revision versions: %s", rev)
			}
			i, err := versions.NextSequence()
			if err != nil {
				return errors.Wrapf(err, "failed to generate sequence number for revision: %s", rev)
			}
			k := [8]byte{}
			binary.BigEndian.PutUint64(k[:], i)
			if err := versions.Put(k[:], uvB); err != nil {
				return errors.Wrap(err, "failed to put revision->version")
			}
		}
		return nil
	})
	if err != nil {
		s.logger.Println(errors.Wrap(err, "failed to cache version map"))
	}
}

func (s *singleSourceCacheBolt) getVersionsFor(rev Revision) (uvs []UnpairedVersion, ok bool) {
	err := s.viewRevBucket(rev, func(b *bolt.Bucket) error {
		versions := cacheFindLatestValid(b, "versions:", s.epoch)
		if versions == nil {
			return nil
		}

		ok = true

		return versions.ForEach(func(_, v []byte) error {
			uv, err := cacheDecodeUnpairedVersion(v)
			if err != nil {
				return err
			}
			uvs = append(uvs, uv)
			return nil
		})
	})
	if err != nil {
		s.logger.Println(errors.Wrapf(err, "failed to get cached versions for revision %q", rev))
		return nil, false
	}
	return
}

func (s *singleSourceCacheBolt) getAllVersions() []PairedVersion {
	var pvs []PairedVersion
	err := s.viewSourceBucket(func(src *bolt.Bucket) error {
		versions := cacheFindLatestValid(src, "versions:", s.epoch)
		if versions == nil {
			return nil
		}

		return versions.ForEach(func(k, v []byte) error {
			uv, err := cacheDecodeUnpairedVersion(k)
			if err != nil {
				return errors.Wrapf(err, "failed to decode unpaired version: %s", k)
			}
			pvs = append(pvs, uv.Pair(Revision(v)))
			return nil
		})
	})
	if err != nil {
		s.logger.Println(errors.Wrap(err, "failed to get all cached versions"))
		return nil
	}
	return pvs
}

func (s *singleSourceCacheBolt) getRevisionFor(uv UnpairedVersion) (rev Revision, ok bool) {
	err := s.viewSourceBucket(func(src *bolt.Bucket) error {
		versions := cacheFindLatestValid(src, "versions:", s.epoch)
		if versions == nil {
			return nil
		}

		k, err := cacheEncodeUnpairedVersion(uv)
		if err != nil {
			return err
		}
		v := versions.Get(k)
		if len(v) > 0 {
			rev = Revision(v)
			ok = true
		}
		return nil
	})
	if err != nil {
		s.logger.Println(errors.Wrapf(err, "failed to get cached revision for unpaired version: %v", uv))
	}
	return
}

func (s *singleSourceCacheBolt) toRevision(v Version) (rev Revision, ok bool) {
	switch t := v.(type) {
	case Revision:
		return t, true
	case PairedVersion:
		return t.Revision(), true
	case UnpairedVersion:
		return s.getRevisionFor(t)
	default:
		s.logger.Println(fmt.Sprintf("failed to get cached revision for version %v: unknown type %T", v, v))
		return "", false
	}
}

func (s *singleSourceCacheBolt) toUnpaired(v Version) (uv UnpairedVersion, ok bool) {
	const errMsg = "failed to get cached unpaired version for version: %v"
	switch t := v.(type) {
	case UnpairedVersion:
		return t, true
	case PairedVersion:
		return t.Unpair(), true
	case Revision:
		err := s.viewRevBucket(t, func(b *bolt.Bucket) error {
			versions := cacheFindLatestValid(b, "versions:", s.epoch)
			if versions == nil {
				return nil
			}

			_, v := versions.Cursor().First()
			if len(v) == 0 {
				return nil
			}
			var err error
			uv, err = cacheDecodeUnpairedVersion(v)
			if err != nil {
				return err
			}

			ok = true
			return nil
		})
		if err != nil {
			s.logger.Println(errors.Wrapf(err, errMsg, v))
		}
		return
	default:
		s.logger.Println(fmt.Sprintf(errMsg, v))
		return
	}
}

// cache revision bucket prefix.
var cacheRev = []byte("rev:")

// cacheRevisionName returns the bucket name for rev.
func cacheRevisionName(rev Revision) []byte {
	name := make([]byte, len(cacheRev)+len(rev))
	copy(name, cacheRev)
	copy(name[len(cacheRev):], string(rev))
	return name
}

// viewSourceBucket executes view with the source bucket, if it exists.
func (s *singleSourceCacheBolt) viewSourceBucket(view func(b *bolt.Bucket) error) error {
	return s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(s.sourceName)
		if b == nil {
			return nil
		}
		return view(b)
	})
}

// updateSourceBucket executes update with the source bucket, creating it first if necessary.
func (s *singleSourceCacheBolt) updateSourceBucket(update func(b *bolt.Bucket) error) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(s.sourceName)
		if err != nil {
			return errors.Wrapf(err, "failed to create bucket: %s", s.sourceName)
		}
		return update(b)
	})
}

// viewRevBucket executes view with rev's bucket for this source, if it exists.
func (s *singleSourceCacheBolt) viewRevBucket(rev Revision, view func(b *bolt.Bucket) error) error {
	return s.viewSourceBucket(func(src *bolt.Bucket) error {
		b := src.Bucket(cacheRevisionName(rev))
		if b == nil {
			return nil
		}
		return view(b)
	})
}

// updateRevBucket executes update with rev's bucket for this source, creating it first if necessary.
func (s *singleSourceCacheBolt) updateRevBucket(rev Revision, update func(b *bolt.Bucket) error) error {
	return s.updateSourceBucket(func(src *bolt.Bucket) error {
		name := cacheRevisionName(rev)
		b, err := src.CreateBucketIfNotExists(name)
		if err != nil {
			return errors.Wrapf(err, "failed to create bucket: %s", name)
		}
		return update(b)
	})
}
