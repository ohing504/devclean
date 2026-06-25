package scanner

// DefaultRegistry returns a registry with all MVP ecosystem scanners.
func DefaultRegistry() *Registry {
	reg := NewRegistry()
	reg.Register(NewNodeScanner())
	reg.Register(NewRustScanner())
	reg.Register(NewRubyScanner())
	reg.Register(NewPythonScanner())
	reg.Register(NewGoScanner())
	reg.Register(NewXcodeScanner())
	reg.Register(NewGlobalScanner())
	return reg
}
