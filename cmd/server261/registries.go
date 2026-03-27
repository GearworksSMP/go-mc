package main

import (
	"github.com/Tnze/go-mc/server"
	"github.com/Tnze/go-mc/server/vanilla"
)

// Wrappers that delegate to the shared vanilla package.
func vanillaRegistryKeys() []server.RegistryKeys { return vanilla.RegistryKeys() }
func vanillaConfigTags() []server.RegistryTagData { return vanilla.ConfigTags() }
