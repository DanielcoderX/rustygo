package basic

type Node struct {
	Value int
}

func LocalNew() int {
	n := new(Node)
	n.Value = 7
	return n.Value
}

func LocalSlice() int {
	buf := make([]byte, 16)
	buf[0] = 3
	return int(buf[0])
}
