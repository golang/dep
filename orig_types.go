package vsolver

type packageName struct {
	name, source, description string
	isRoot, isMagic           bool
}

type packageRef packageName

type packageID struct {
	packageName
	version string
}

type packageDep struct {
	packageName
	constraint versionConstraint
}

type versionSelection struct {
	s   solver
	ids []packageID
	//deps map[string][]dependency
	deps  map[packageRef][]dependency
	unsel unselectedPackageQueue
}

type versionConstraint interface{}
type versionRange struct{}
type emptyVersion struct{}

type dependency struct {
	depender packageID
	dep      packageDep
}

type unselectedPackageQueue struct {
	s solver
	q pqueue
}

func (upq unselectedPackageQueue) First() packageRef {

}

type pqueue []packageName // TODO adapt semver sorting to create a priority queue/heap
