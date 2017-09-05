// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/boltdb/bolt"
	"github.com/golang/dep/internal/gps/pkgtree"
	"github.com/pkg/errors"
)

// cacheEncodeUnpairedVersion returns an encoded UnpairedVersion.
func cacheEncodeUnpairedVersion(uv UnpairedVersion) ([]byte, error) {
	var pre string
	switch uv.Type() {
	case IsBranch:
		if uv.(branchVersion).isDefault {
			pre = "defaultBranch:"
		} else {
			pre = "branch:"
		}
	case IsSemver, IsVersion:
		pre = "ver:"
	default:
		return nil, fmt.Errorf("unrecognized version type: %d", uv.Type())
	}
	return []byte(pre + uv.String()), nil
}

// cacheDecodeUnpairedVersion decodes and returns a new UnpairedVersion.
func cacheDecodeUnpairedVersion(b []byte) (UnpairedVersion, error) {
	const br, dbr, ver = "branch:", "defaultBranch:", "ver:"
	s := string(b)
	switch {
	case strings.HasPrefix(s, br):
		return NewBranch(strings.TrimPrefix(s, br)), nil
	case strings.HasPrefix(s, dbr):
		return newDefaultBranch(strings.TrimPrefix(s, dbr)), nil
	case strings.HasPrefix(s, ver):
		return NewVersion(strings.TrimPrefix(s, ver)), nil
	default:
		return nil, fmt.Errorf("unrecognized prefix: %s", s)
	}
}

// cacheDecodeProjectProperties returns a new ProjectRoot and ProjectProperties with the
// data encoded in the key/value pair.
func cacheDecodeProjectProperties(k, v []byte) (ProjectRoot, ProjectProperties, error) {
	var pp ProjectProperties
	ks := strings.SplitN(string(k), ",", 2)
	ip := ProjectRoot(ks[0])
	if len(ks) > 1 {
		pp.Source = ks[1]
	}
	if len(v) == 0 {
		pp.Constraint = Any()
	} else {
		const br, dbr, ver, rev = "branch:", "defaultBranch:", "ver:", "rev:"
		vs := string(v)
		switch {
		case strings.HasPrefix(vs, br):
			pp.Constraint = NewBranch(strings.TrimPrefix(vs, br))

		case strings.HasPrefix(vs, dbr):
			pp.Constraint = newDefaultBranch(strings.TrimPrefix(vs, dbr))

		case strings.HasPrefix(vs, ver):
			vs = strings.TrimPrefix(vs, ver)
			if c, err := NewSemverConstraint(vs); err != nil {
				pp.Constraint = NewVersion(vs)
			} else {
				pp.Constraint = c
			}

		case strings.HasPrefix(vs, rev):
			pp.Constraint = Revision(strings.TrimPrefix(vs, rev))

		default:
			return "", ProjectProperties{}, fmt.Errorf("unrecognized prefix: %s", vs)
		}
	}

	return ip, pp, nil
}

// cacheEncodeProjectProperties returns a key/value pair containing the encoded
// ProjectRoot and ProjectProperties.
func cacheEncodeProjectProperties(ip ProjectRoot, pp ProjectProperties) ([]byte, []byte, error) {
	k := string(ip)
	if len(pp.Source) > 0 {
		k += "," + pp.Source
	}
	if pp.Constraint == nil || IsAny(pp.Constraint) {
		return []byte(k), []byte{}, nil
	}

	if v, ok := pp.Constraint.(Version); ok {
		var val string
		switch v.Type() {
		case IsRevision:
			val = "rev:" + v.String()
		case IsBranch:
			if v.(branchVersion).isDefault {
				val = "defaultBranch:" + v.String()
			} else {
				val = "branch:" + v.String()
			}
		case IsSemver, IsVersion:
			val = "ver:" + v.String()
		default:
			return nil, nil, fmt.Errorf("unrecognized VersionType: %v", v.Type())
		}
		return []byte(k), []byte(val), nil
	}

	// Has to be a semver range.
	v := pp.Constraint.String()
	return []byte(k), []byte("ver:" + v), nil
}

// cachePutManifest stores a Manifest in the bolt.Bucket.
func cachePutManifest(b *bolt.Bucket, m Manifest) error {
	// Constraints
	cs, err := b.CreateBucket([]byte("cs"))
	if err != nil {
		return err
	}
	for ip, pp := range m.DependencyConstraints() {
		k, v, err := cacheEncodeProjectProperties(ip, pp)
		if err != nil {
			return err
		}
		if err := cs.Put(k, v); err != nil {
			return err
		}
	}

	rm, ok := m.(RootManifest)
	if !ok {
		return nil
	}

	// Ignored
	var igPkgs []string
	for ip, ok := range rm.IgnoredPackages() {
		if ok {
			igPkgs = append(igPkgs, ip)
		}
	}
	if len(igPkgs) > 0 {
		v := []byte(strings.Join(igPkgs, ","))
		if err := b.Put([]byte("ig"), v); err != nil {
			return err
		}
	}

	// Overrides
	ovr, err := b.CreateBucket([]byte("ovr"))
	if err != nil {
		return err
	}
	for ip, pp := range rm.Overrides() {
		k, v, err := cacheEncodeProjectProperties(ip, pp)
		if err != nil {
			return err
		}
		if err := ovr.Put(k, v); err != nil {
			return err
		}
	}

	// Required
	var reqPkgs []string
	for ip, ok := range rm.RequiredPackages() {
		if ok {
			reqPkgs = append(reqPkgs, ip)
		}
	}
	if len(reqPkgs) > 0 {
		v := []byte(strings.Join(reqPkgs, ","))
		if err := b.Put([]byte("req"), v); err != nil {
			return err
		}
	}

	return nil
}

// cacheGetManifest returns a new RootManifest with the data retrieved from the bolt.Bucket.
func cacheGetManifest(b *bolt.Bucket) (RootManifest, error) {
	m := &simpleRootManifest{
		c:   make(ProjectConstraints),
		ovr: make(ProjectConstraints),
		ig:  make(map[string]bool),
		req: make(map[string]bool),
	}

	// Constraints
	if cs := b.Bucket([]byte("cs")); cs != nil {
		err := cs.ForEach(func(k, v []byte) error {
			ip, pp, err := cacheDecodeProjectProperties(k, v)
			if err != nil {
				return err
			}
			m.c[ip] = pp
			return nil
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to get constraints")
		}
	}

	// Ignored
	if ig := b.Get([]byte("ig")); len(ig) > 0 {
		for _, ip := range splitString(string(ig), ",") {
			m.ig[ip] = true
		}
	}

	// Overrides
	if os := b.Bucket([]byte("ovr")); os != nil {
		err := os.ForEach(func(k, v []byte) error {
			ip, pp, err := cacheDecodeProjectProperties(k, v)
			if err != nil {
				return err
			}
			m.ovr[ip] = pp
			return nil
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to get overrides")
		}
	}

	// Required
	if req := b.Get([]byte("req")); len(req) > 0 {
		for _, ip := range splitString(string(req), ",") {
			m.req[ip] = true
		}
	}

	return m, nil
}

// cachePutLockedProject stores the LockedProject as fields in the bolt.Bucket.
func cachePutLockedProject(b *bolt.Bucket, lp LockedProject) error {
	rev, branch, ver := VersionComponentStrings(lp.Version())
	for _, field := range []struct{ k, v string }{
		{"branch", branch},
		{"pkgs", strings.Join(lp.pkgs, ",")},
		{"rev", rev},
		{"src", string(lp.Ident().Source)},
		{"ver", ver},
	} {
		if len(field.v) > 0 {
			if err := b.Put([]byte(field.k), []byte(field.v)); err != nil {
				return errors.Wrap(err, "failed to put locked project")
			}
		}
	}
	return nil
}

// cacheGetLockedProject returns a new LockedProject with fields from the bolt.Bucket.
func cacheGetLockedProject(b *bolt.Bucket) (lp LockedProject, err error) {
	br := string(b.Get([]byte("branch")))
	pkgs := splitString(string(b.Get([]byte("pkgs"))), ",")
	r := string(b.Get([]byte("rev")))
	pi := ProjectIdentifier{Source: string(b.Get([]byte("src")))}
	v := string(b.Get([]byte("ver")))

	var ver Version = Revision(r)
	if v != "" {
		if br != "" {
			err = errors.New("both branch and version specified")
			return
		}
		ver = NewVersion(v).Pair(Revision(r))
	} else if br != "" {
		ver = NewBranch(br).Pair(Revision(r))
	} else if r == "" {
		err = errors.New("no branch, version, or revision")
		return
	}

	lp = NewLockedProject(pi, ver, pkgs)
	return
}

// cachePutLock stores the Lock as fields in the bolt.Bucket.
func cachePutLock(b *bolt.Bucket, l Lock) error {
	// InputHash
	if v := l.InputHash(); len(v) > 0 {
		if err := b.Put([]byte("hash"), v); err != nil {
			return errors.Wrap(err, "failed to put hash")
		}
	}

	// Projects
	if len(l.Projects()) > 0 {
		for _, lp := range l.Projects() {
			lb, err := b.CreateBucket([]byte("lock:" + lp.pi.ProjectRoot))
			if err != nil {
				return errors.Wrapf(err, "failed to create bucket for project identifier: %v", lp.pi)
			}
			if err := cachePutLockedProject(lb, lp); err != nil {
				return err
			}
		}
	}

	return nil
}

// cacheGetLock returns a new *safeLock with the fields retrieved from the bolt.Bucket.
func cacheGetLock(b *bolt.Bucket) (*safeLock, error) {
	l := &safeLock{
		h: b.Get([]byte("hash")),
	}
	c := b.Cursor()
	p := []byte("lock:")
	for k, _ := c.Seek(p); bytes.HasPrefix(k, p); k, _ = c.Next() {
		lp, err := cacheGetLockedProject(b.Bucket(k))
		if err != nil {
			return nil, errors.Wrap(err, "failed to get lock")
		}
		lp.pi.ProjectRoot = ProjectRoot(bytes.TrimPrefix(k, p))
		l.p = append(l.p, lp)
	}
	return l, nil
}

// cachePutPackageOrError stores the pkgtree.PackageOrErr as fields in the bolt.Bucket.
func cachePutPackageOrErr(b *bolt.Bucket, poe pkgtree.PackageOrErr) error {
	if poe.Err != nil {
		err := b.Put([]byte("err"), []byte(poe.Err.Error()))
		if err != nil {
			return errors.Wrapf(err, "failed to put error: %v", poe.Err)
		}
	} else {
		for _, f := range []struct{ k, v string }{
			{"cp", poe.P.CommentPath},
			{"ip", strings.Join(poe.P.Imports, ",")},
			{"nm", poe.P.Name},
			{"tip", strings.Join(poe.P.TestImports, ",")},
		} {
			if len(f.v) > 0 {
				err := b.Put([]byte(f.k), []byte(f.v))
				if err != nil {
					return errors.Wrapf(err, "failed to put package: %v", poe.P)
				}
			}
		}
	}
	return nil
}

// cacheGetPackageOrErr returns a new pkgtree.PackageOrErr with fields retrieved
// from the bolt.Bucket.
func cacheGetPackageOrErr(b *bolt.Bucket) pkgtree.PackageOrErr {
	if v := b.Get([]byte("err")); len(v) > 0 {
		return pkgtree.PackageOrErr{
			Err: errors.New(string(v)),
		}
	}
	return pkgtree.PackageOrErr{
		P: pkgtree.Package{
			CommentPath: string(b.Get([]byte("cp"))),
			Imports:     splitString(string(b.Get([]byte("ip"))), ","),
			Name:        string(b.Get([]byte("nm"))),
			TestImports: splitString(string(b.Get([]byte("tip"))), ","),
		},
	}
}

//cacheTimestampedKey returns a prefixed key with a trailing timestamp
func cacheTimestampedKey(pre string, t time.Time) []byte {
	b := make([]byte, len(pre)+8)
	copy(b, pre)
	binary.BigEndian.PutUint64(b[len(pre):], uint64(t.Unix()))
	return b
}

// boltTxOrBucket is a minimal interface satisfied by bolt.Tx and bolt.Bucket.
type boltTxOrBucket interface {
	Cursor() *bolt.Cursor
	DeleteBucket([]byte) error
	Bucket([]byte) *bolt.Bucket
}

// cachePrefixDelete prefix scans and deletes each bucket.
func cachePrefixDelete(tob boltTxOrBucket, pre string) error {
	c := tob.Cursor()
	p := []byte(pre)
	for k, _ := c.Seek(p); bytes.HasPrefix(k, p); k, _ = c.Next() {
		if err := tob.DeleteBucket(k); err != nil {
			return errors.Wrapf(err, "failed to delete bucket: %s", k)
		}
	}
	return nil
}

// cacheFindLatestValid prefix scans for the latest bucket which is timestamped >= epoch,
// or returns nil if none exists.
func cacheFindLatestValid(tob boltTxOrBucket, pre string, epoch int64) *bolt.Bucket {
	c := tob.Cursor()
	p := []byte(pre)
	var latest []byte
	for k, _ := c.Seek(p); bytes.HasPrefix(k, p); k, _ = c.Next() {
		latest = k
	}
	if latest == nil {
		return nil
	}
	ts := bytes.TrimPrefix(latest, p)
	if len(ts) != 8 {
		return nil
	}
	if int64(binary.BigEndian.Uint64(ts)) < epoch {
		return nil
	}
	return tob.Bucket(latest)
}

// splitString delegates to strings.Split, but returns nil in place of a single empty element.
func splitString(s, sep string) []string {
	r := strings.Split(s, sep)
	if len(r) == 1 && r[0] == "" {
		return nil
	}
	return r
}
