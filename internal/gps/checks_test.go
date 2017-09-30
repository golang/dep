package gps

import (
	"reflect"
	"sort"
	"testing"
)

func TestBasicFindIneffectualConstraints(t *testing.T) {
	if fixtorun != "" {
		if fix, exists := basicFixtures[fixtorun]; exists {
			runBasicTest1(fix, t)
		}
	} else {
		// sort them by their keys so we get stable output
		var names []string
		for n := range basicFixtures {
			names = append(names, n)
		}

		sort.Strings(names)
		for _, n := range names {
			t.Run(n, func(t *testing.T) {
				runBasicTest1(basicFixtures[n], t)
			})
		}
	}
}

func runBasicTest1(fix basicFixture, t *testing.T) {
	ineffectuals := FindIneffectualConstraints(fix.rootmanifest(), fix.rootTree())
	if !reflect.DeepEqual(ineffectuals, fix.ineffectuals) {
		t.Errorf("FindIneffectualConstraints expected:\n\t(GOT) %v\n\t(WNT) %v", ineffectuals, fix.ineffectuals)
	}
}

func TestBimodalFindIneffectualConstraints(t *testing.T) {
	if fixtorun != "" {
		if fix, exists := bimodalFixtures[fixtorun]; exists {
			runBimodalTest1(fix, t)
		}
	} else {
		// sort them by their keys so we get stable output
		var names []string
		for n := range bimodalFixtures {
			names = append(names, n)
		}

		sort.Strings(names)
		for _, n := range names {
			t.Run(n, func(t *testing.T) {
				runBimodalTest1(bimodalFixtures[n], t)
			})
		}
	}
}

func runBimodalTest1(fix bimodalFixture, t *testing.T) {
	ineffectuals := FindIneffectualConstraints(fix.rootmanifest(), fix.rootTree())
	if !reflect.DeepEqual(ineffectuals, fix.ineffectuals) {
		t.Errorf("FindIneffectualConstraints expected:\n\t(GOT) %v\n\t(WNT) %v", ineffectuals, fix.ineffectuals)
	}
}
