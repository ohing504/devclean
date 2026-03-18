package scanner

// DefaultRegistry returns a registry with all MVP ecosystem scanners.
func DefaultRegistry() *Registry {
	reg := NewRegistry()
	reg.Register(NewNodeScanner())
	reg.Register(NewRustScanner())
	return reg
}
