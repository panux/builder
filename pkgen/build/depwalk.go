package build

// DepWalker is a function to walk a package dependency tree.
type DepWalker func(string) ([]string, error)

// Walk walks the dependency tree.
func (dw DepWalker) Walk(pkgs ...string) ([]string, error) {
	// create walk tracker
	ds := &depSet{
		depscan: make(map[string]struct{}),
		lst:     []string{},
		walker:  dw,
	}

	// walk dependencies
	for _, v := range pkgs {
		err := ds.walk(v)
		if err != nil {
			return nil, err
		}
	}

	// use list from depSet
	return ds.lst, nil
}

// depSet is a set of dependencies used to walk dependencies.
type depSet struct {
	depscan map[string]struct{}
	lst     []string
	walker  DepWalker
}

// walk runs a recursive dependency walk.
func (ds *depSet) walk(pkgname string) error {
	// dont rescan a dependency
	if _, ok := ds.depscan[pkgname]; ok {
		return nil
	}
	ds.depscan[pkgname] = struct{}{}

	// get dependencies
	deps, err := ds.walker(pkgname)
	if err != nil {
		return err
	}

	// recurse dependencies
	for _, v := range deps {
		err = ds.walk(v)
		if err != nil {
			return err
		}
	}

	// add package to list
	ds.lst = append(ds.lst, pkgname)

	return nil
}
