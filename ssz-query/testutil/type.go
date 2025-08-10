package testutil

type PathTest struct {
	Path     string
	Expected any
}

type TestSpec struct {
	Name      string
	Instance  any
	PathTests []PathTest
}
