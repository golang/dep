// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"github.com/pelletier/go-toml"
	"github.com/pkg/errors"
)

type tomlMapper struct {
	Tree  *toml.TomlTree
	Error error
}

func readTableAsProjects(mapper *tomlMapper, table string) []rawProject {
	if mapper.Error != nil { // Stop mapping if an error has already occurred
		return nil
	}

	query, err := mapper.Tree.Query("$." + table)
	if err != nil {
		mapper.Error = errors.Wrapf(err, "Unable to query for [[%s]]", table)
		return nil
	}

	matches := query.Values()
	if len(matches) == 0 {
		return nil
	}

	tables, ok := matches[0].([]*toml.TomlTree)
	if !ok {
		mapper.Error = errors.Errorf("Invalid query result type for [[%s]], should be a TOML array of tables but got %T", table, matches[0])
		return nil
	}

	subMapper := &tomlMapper{}
	projects := make([]rawProject, len(tables))
	for i := 0; i < len(tables); i++ {
		subMapper.Tree = tables[i]
		projects[i] = mapProject(subMapper)
	}

	if subMapper.Error != nil {
		mapper.Error = subMapper.Error
		return nil
	}
	return projects
}

func readTableAsLockedProjects(mapper *tomlMapper, table string) []rawLockedProject {
	if mapper.Error != nil { // Stop mapping if an error has already occurred
		return nil
	}

	query, err := mapper.Tree.Query("$." + table)
	if err != nil {
		mapper.Error = errors.Wrapf(err, "Unable to query for [[%s]]", table)
		return nil
	}

	matches := query.Values()
	if len(matches) == 0 {
		return nil
	}

	tables, ok := matches[0].([]*toml.TomlTree)
	if !ok {
		mapper.Error = errors.Errorf("Invalid query result type for [[%s]], should be a TOML array of tables but got %T", table, matches[0])
		return nil
	}

	subMapper := &tomlMapper{}
	projects := make([]rawLockedProject, len(tables))
	for i := 0; i < len(tables); i++ {
		subMapper.Tree = tables[i]
		projects[i] = mapLockedProject(subMapper)
	}

	if subMapper.Error != nil {
		mapper.Error = subMapper.Error
		return nil
	}
	return projects
}

func mapProject(mapper *tomlMapper) rawProject {
	if mapper.Error != nil { // Stop mapping if an error has already occurred
		return rawProject{}
	}

	prj := rawProject{
		Name:     readKeyAsString(mapper, "name"),
		Branch:   readKeyAsString(mapper, "branch"),
		Revision: readKeyAsString(mapper, "revision"),
		Version:  readKeyAsString(mapper, "version"),
		Source:   readKeyAsString(mapper, "source"),
	}

	if mapper.Error != nil {
		return rawProject{}
	}

	return prj
}

func mapLockedProject(mapper *tomlMapper) rawLockedProject {
	if mapper.Error != nil { // Stop mapping if an error has already occurred
		return rawLockedProject{}
	}

	prj := rawLockedProject{
		Name:     readKeyAsString(mapper, "name"),
		Branch:   readKeyAsString(mapper, "branch"),
		Revision: readKeyAsString(mapper, "revision"),
		Version:  readKeyAsString(mapper, "version"),
		Source:   readKeyAsString(mapper, "source"),
		Packages: readKeyAsStringList(mapper, "packages"),
	}

	if mapper.Error != nil {
		return rawLockedProject{}
	}

	return prj
}

func readKeyAsString(mapper *tomlMapper, key string) string {
	if mapper.Error != nil { // Stop mapping if an error has already occurred
		return ""
	}

	rawValue := mapper.Tree.GetDefault(key, "")
	value, ok := rawValue.(string)
	if !ok {
		mapper.Error = errors.Errorf("Invalid type for %s, should be a string, but it is a %T", key, rawValue)
		return ""
	}

	return value
}

func readKeyAsStringList(mapper *tomlMapper, key string) []string {
	if mapper.Error != nil { // Stop mapping if an error has already occurred
		return nil
	}

	query, err := mapper.Tree.Query("$." + key)
	if err != nil {
		mapper.Error = errors.Wrapf(err, "Unable to query for [%s]", key)
		return nil
	}

	matches := query.Values()
	if len(matches) == 0 {
		return nil
	}

	lists, ok := matches[0].([]interface{})
	if !ok {
		mapper.Error = errors.Errorf("Invalid query result type for [%s], should be a TOML list ([]interface{}) but got %T", key, matches[0])
		return nil
	}

	results := make([]string, len(lists))
	for i := range lists {
		ref, ok := lists[i].(string)
		if !ok {
			mapper.Error = errors.Errorf("Invalid query result item type for [%s], should be a TOML list of strings([]string) but got %T", key, lists[i])
			return nil
		}
		results[i] = ref
	}
	return results
}
