package scanner

// DefaultRegistry returns a registry with all MVP ecosystem scanners.
func DefaultRegistry() *Registry {
	reg := NewRegistry()
	reg.Register(newWalkScanner(nodeWalkEcosystem))
	reg.Register(newWalkScanner(rustWalkEcosystem))
	reg.Register(newWalkScanner(rubyWalkEcosystem))
	reg.Register(NewPythonScanner())
	reg.Register(newWalkScanner(goWalkEcosystem))
	reg.Register(NewXcodeScanner())
	reg.Register(NewGlobalScanner())
	reg.Register(NewLLMScanner())
	return reg
}
