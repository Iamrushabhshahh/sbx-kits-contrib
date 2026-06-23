package v1migrate

import "github.com/docker/sbx-kits-contrib/spec"

// This package is a frozen, self-contained copy of the former v1-aware loader.
// It reuses the spec package's EXPORTED domain types via these aliases so the
// copied decode/normalize code compiles unchanged while sharing spec's actual
// types (no domain-type drift). Only the unexported decode machinery (specFile,
// the polymorphic wrappers, warnings, the normalize passes) is duplicated here;
// those are what understand v1. The whole package is a clean delete when v1 is
// removed from the spec.
type (
	Artifact          = spec.Artifact
	Manifest          = spec.Manifest
	MountSpec         = spec.MountSpec
	PublishedPort     = spec.PublishedPort
	NetworkPolicy     = spec.NetworkPolicy
	ServiceAuth       = spec.ServiceAuth
	EnvironmentPolicy = spec.EnvironmentPolicy
	SettingsPolicy    = spec.SettingsPolicy
	Caps              = spec.Caps
	CapsNetwork       = spec.CapsNetwork
	OAuthPolicy       = spec.OAuthPolicy
	OAuth             = spec.OAuth
	CommandsPolicy    = spec.CommandsPolicy
	Credential        = spec.Credential
	CredentialSource  = spec.CredentialSource
	ApiKey            = spec.ApiKey
	ApiKeyInject      = spec.ApiKeyInject
	BuildConfig       = spec.BuildConfig
	Resources         = spec.Resources
	Security          = spec.Security
)

const (
	KindAgent      = spec.KindAgent
	KindSandbox    = spec.KindSandbox
	MountTypeTmpfs = spec.MountTypeTmpfs
)
