package buildmanager

//DepWalker is a function to walk a package dependency tree
type DepWalker func(string) ([]string, error)

//Walk walks the dependency tree
func (dw DepWalker) Walk(pkgs ...string) ([]string, error) {
	ds := &depSet{
		depscan: make(map[string]bool),
		lst:     []string{},
		walker:  dw,
	}
	for _, v := range pkgs {
		err := ds.walk(v)
		if err != nil {
			return nil, err
		}
	}
	return ds.lst, nil
}

type depSet struct {
	depscan map[string]bool
	lst     []string
	walker  DepWalker
}

func (ds *depSet) walk(pkgname string) error {
	if ds.depscan[pkgname] {
		return nil
	}
	ds.depscan[pkgname] = true
	deps, err := ds.walker(pkgname)
	if err != nil {
		return err
	}
	for _, v := range deps {
		err = ds.walk(v)
		if err != nil {
			return err
		}
	}
	ds.lst = append(ds.lst, pkgname)
	return nil
}
