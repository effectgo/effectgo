package badmap

//effectgo:errmap
func badSig(s string) error { return nil } // want "must have signature func\\(error\\) error" badSig:"."
