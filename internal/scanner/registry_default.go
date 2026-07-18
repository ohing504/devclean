package scanner

// DefaultRegistry returns a registry with all MVP ecosystem scanners.
func DefaultRegistry() *Registry {
	reg := NewRegistry()
	reg.Register(newWalkScanner(nodeWalkEcosystem))
	reg.Register(newWalkScanner(rustWalkEcosystem))
	reg.Register(newWalkScanner(rubyWalkEcosystem))
	reg.Register(newWalkScanner(pythonWalkEcosystem))
	reg.Register(newWalkScanner(goWalkEcosystem))
	reg.Register(newWalkScanner(flutterWalkEcosystem))
	reg.Register(newWalkScanner(androidWalkEcosystem))
	reg.Register(NewXcodeScanner())
	reg.Register(NewDockerScanner())
	reg.Register(NewGlobalScanner())
	reg.Register(NewLLMScanner())
	return reg
}
