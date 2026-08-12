package rewrite

// RewriteOptions contains configuration flags for the arena rewrite pass.
type RewriteOptions struct {
	ArenaBytes int
	PkgPath    string
}

// RewriteResult holds the modified source bytes and statistics.
type RewriteResult struct {
	Content        []byte
	Changed        bool
	RewrittenCount int
}
