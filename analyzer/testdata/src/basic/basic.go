package basic

type Node struct {
	Val  int
	Next *Node
}

// newLocal: result stays inside function — arena-eligible
func newLocal() int {
	n := new(Node) // want `arena-eligible`
	n.Val = 42
	return n.Val
}

// newEscapes: result returned — escapes, NOT eligible
func newEscapes() *Node {
	n := new(Node)
	return n
}

// makeLocal: slice stays inside function — arena-eligible
func makeLocal() int {
	s := make([]int, 64) // want `arena-eligible`
	s[0] = 1
	return s[0]
}

// makeEscapes: slice returned — escapes, NOT eligible
func makeEscapes() []int {
	s := make([]int, 64)
	return s
}

// compositeLitLocal: struct literal stays local — arena-eligible
func compositeLitLocal() int {
	n := Node{Val: 1} // want `arena-eligible`
	return n.Val
}

type Holder struct {
	Node *Node
}

// newEscapesViaCompositeLiteral: pointer stored into struct literal â€” escapes, NOT eligible
func newEscapesViaCompositeLiteral() *Node {
	n := new(Node)
	return (Holder{Node: n}).Node
}

// newEscapesViaSliceLiteral: pointer stored into slice literal â€” escapes, NOT eligible
func newEscapesViaSliceLiteral() *Node {
	n := new(Node)
	nodes := []*Node{n}
	return nodes[0]
}

// literalEscapesViaFieldAddress: taking address of a field aliases the struct storage â€” escapes, NOT eligible
func literalEscapesViaFieldAddress() *int {
	n := Node{Val: 7}
	return &n.Val
}

// makeEscapesViaIndexAddress: taking address of an indexed element aliases the slice backing storage â€” escapes, NOT eligible
func makeEscapesViaIndexAddress() *int {
	s := make([]int, 4)
	return &s[0]
}
