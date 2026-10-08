package iface

import "io"

//effectgo:pure
func Write(w io.Writer) error { // want Write:`map\[unknown:\[\{iface iface Write\} \{io io Writer\.Write\}\]\]` `Write is declared //effectgo:pure but has effect unknown via Write → io\.Writer\.Write`
	_, err := w.Write(nil)
	return err
}

//effectgo:allow unknown
func WriteOK(w io.Writer) error { // want WriteOK:`map\[unknown:`
	_, err := w.Write(nil)
	return err
}

//effectgo:allow time
func WriteBad(w io.Writer) error { // want WriteBad:`map\[unknown:` `WriteBad is declared //effectgo:allow time but has effect unknown via WriteBad → io\.Writer\.Write`
	_, err := w.Write(nil)
	return err
}

//effectgo:pure
func CallFunc(f func() int) int { // want CallFunc:`map\[param:0:`
	return f()
}
