package pipe

type Op func() error

func Chain(ops []Op) error {
	for _, op := range ops {
		err := op()
		if err != nil {
			return err
		}
	}
	return nil
}

func ErrIf(cond bool, err error) error {
	if cond {
		return err
	}
	return nil
}
