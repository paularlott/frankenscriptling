package frankenscriptling

// Build identity reported by Scriptling::getBuildInfo(). These are the dev
// placeholders for a checkout; the Dockerfile overwrites this file with the
// real image version, git revision and the FrankenPHP and PHP versions it
// builds against. The scriptling version comes from the module itself.

const (
	imageVersion      = "0.0.0-dev"
	imageRevision     = "unknown"
	frankenPHPVersion = "unknown"
	phpBuildVersion   = "unknown"
)
