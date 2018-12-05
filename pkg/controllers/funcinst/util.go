package funcinst

func retryOnceOnError(fn func() error) error {
	for i := 0; ; i++ {
		err := fn()
		if err != nil {
			if i >= 1 {
				return err
			}
			continue
		}
		return nil
	}
}
