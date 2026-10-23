# =============================================================================
# Variables: no defaults here on purpose. The Makefile is the one place that
# sets versions and tags and exports them; a bare `docker buildx bake` fails
# with the messages below instead of silently building stale versions.
# =============================================================================

variable "TAG_BASE" {
  validation {
    condition     = TAG_BASE != ""
    error_message = "TAG_BASE is not set: build through the Makefile, which exports it."
  }
}

variable "CACHE_TAG_BASE" {}

variable "FRANKENPHP_VERSION" {
  validation {
    condition     = FRANKENPHP_VERSION != ""
    error_message = "FRANKENPHP_VERSION is not set: build through the Makefile, which exports it."
  }
}

variable "SCRIPTLING_VERSION" {
  validation {
    condition     = SCRIPTLING_VERSION != ""
    error_message = "SCRIPTLING_VERSION is not set: build through the Makefile, which exports it."
  }
}

variable "PHP_VERSIONS" {
  type = list(string)
  validation {
    condition     = length(PHP_VERSIONS) > 0
    error_message = "PHP_VERSIONS is not set: build through the Makefile, which exports it as a comma-separated list."
  }
}

# The newest PHP line, which also gets the floating `latest` tag.
variable "PHP_LATEST" {
  validation {
    condition     = PHP_LATEST != ""
    error_message = "PHP_LATEST is not set: build through the Makefile, which exports it."
  }
}

# The image's own version (the git tag, without its v) and git revision.
variable "VERSION" {
  validation {
    condition     = VERSION != ""
    error_message = "VERSION is not set: build through the Makefile, which derives it from the git tag."
  }
}

variable "REVISION" {
  validation {
    condition     = REVISION != ""
    error_message = "REVISION is not set: build through the Makefile, which derives it from git."
  }
}

# =============================================================================
# Helper functions
# =============================================================================

function "cache_base" {
  params = []
  result = CACHE_TAG_BASE == "" ? TAG_BASE : CACHE_TAG_BASE
}

function "major_minor" {
  params = [version]
  result = "${split(".", version)[0]}.${split(".", version)[1]}"
}

function "no_prefix" {
  params = [version]
  result = trimprefix(version, "v")
}

# Dev builds (0.0.0-dev.<sha>, see the Makefile) never move the floating tags.
function "is_release" {
  params = []
  result = length(regexall("^0\\.0\\.0-dev", VERSION)) == 0
}

# =============================================================================
# Shared target
# =============================================================================

target "_common" {
  platforms = ["linux/amd64", "linux/arm64"]
  output    = [{ type = "image", push = true }]

  labels = {
    "org.opencontainers.image.vendor" = "Paul Arlott"
    "org.opencontainers.image.source" = "https://github.com/paularlott/frankenscriptling"
  }
}

# =============================================================================
# Groups
# =============================================================================

group "default" {
  targets = ["frankenscriptling", "frankenscriptling-builder"]
}

# =============================================================================
# Targets
# =============================================================================

target "frankenscriptling" {
  name        = "frankenscriptling-${replace(php, ".", "-")}"
  description = "FrankenPHP with the Scriptling PHP extension"
  matrix      = { php = PHP_VERSIONS }
  inherits    = ["_common"]
  context     = "."
  # Explicit, not "whichever stage happens to be last in the Dockerfile" —
  # the Dockerfile also has test-only stages (test-plugin/test-plugin-export)
  # that must never become the published image by accident.
  target      = "runtime"

  # The image version is the tag; what went into it is recorded as labels
  # (docker inspect) and reported by Scriptling::getBuildInfo() from inside.
  labels = {
    "org.opencontainers.image.title"       = "Frankenscriptling"
    "org.opencontainers.image.description" = "FrankenPHP with the Scriptling PHP extension"
    "org.opencontainers.image.version"     = "${VERSION}"
    "org.opencontainers.image.revision"    = "${REVISION}"
    "org.opencontainers.image.created"     = "${timestamp()}"
    "org.opencontainers.image.url"         = "https://hub.docker.com/r/paularlott/frankenscriptling"
    "org.opencontainers.image.documentation" = "https://github.com/paularlott/frankenscriptling#readme"
    "dev.scriptling.scriptling-version"    = "${no_prefix(SCRIPTLING_VERSION)}"
    "dev.scriptling.frankenphp-version"    = "${FRANKENPHP_VERSION}"
    "dev.scriptling.php-version"           = "${php}"
  }

  args = {
    FRANKENPHP_VERSION = "${FRANKENPHP_VERSION}"
    PHP_VERSION        = "${php}"
    SCRIPTLING_VERSION = "${SCRIPTLING_VERSION}"
    IMAGE_VERSION      = "${VERSION}"
    IMAGE_REVISION     = "${REVISION}"
  }

  # <version>-php<x.y.z> and <version>-php<x.y> are immutable: a release is
  # built once. php<x.y> floats to the newest release for that PHP line, and
  # latest floats to the newest release on the newest PHP line.
  tags = concat(
    [
      "${TAG_BASE}/frankenscriptling:${VERSION}-php${php}",
      "${TAG_BASE}/frankenscriptling:${VERSION}-php${major_minor(php)}",
    ],
    is_release() ? ["${TAG_BASE}/frankenscriptling:php${major_minor(php)}"] : [],
    is_release() && php == PHP_LATEST ? ["${TAG_BASE}/frankenscriptling:latest"] : [],
  )

  cache-from = [{
    type = "registry"
    ref  = "${cache_base()}/frankenscriptling:buildcache-php${php}"
  }]
  cache-to = [{
    type              = "registry"
    ref               = "${cache_base()}/frankenscriptling:buildcache-php${php}"
    mode              = "max"
    "oci-media-types" = true
    "image-manifest"  = true
  }]
}

# The builder stage: libphp, xcaddy, the prepared extension source and
# build-frankenscriptling, so downstream images can rebuild the binary with
# extra Caddy modules. Tagged the way FrankenPHP tags its builders
# (<version>-builder-php<php>, builder-php<x.y>, builder) and pulls the
# runtime's cache (its first stages are this stage).
target "frankenscriptling-builder" {
  name        = "frankenscriptling-builder-${replace(php, ".", "-")}"
  description = "Builder for FrankenPHP with the Scriptling PHP extension and extra Caddy modules"
  matrix      = { php = PHP_VERSIONS }
  inherits    = ["_common"]
  context     = "."
  target      = "builder"

  labels = {
    "org.opencontainers.image.title"       = "Frankenscriptling builder"
    "org.opencontainers.image.description" = "Builds FrankenPHP with the Scriptling PHP extension and extra Caddy modules"
    "org.opencontainers.image.version"     = "${VERSION}"
    "org.opencontainers.image.revision"    = "${REVISION}"
    "org.opencontainers.image.created"     = "${timestamp()}"
    "dev.scriptling.scriptling-version"    = "${no_prefix(SCRIPTLING_VERSION)}"
    "dev.scriptling.frankenphp-version"    = "${FRANKENPHP_VERSION}"
    "dev.scriptling.php-version"           = "${php}"
  }

  args = {
    FRANKENPHP_VERSION = "${FRANKENPHP_VERSION}"
    PHP_VERSION        = "${php}"
    SCRIPTLING_VERSION = "${SCRIPTLING_VERSION}"
    IMAGE_VERSION      = "${VERSION}"
    IMAGE_REVISION     = "${REVISION}"
  }

  tags = concat(
    [
      "${TAG_BASE}/frankenscriptling:${VERSION}-builder-php${php}",
      "${TAG_BASE}/frankenscriptling:${VERSION}-builder-php${major_minor(php)}",
    ],
    is_release() ? ["${TAG_BASE}/frankenscriptling:builder-php${major_minor(php)}"] : [],
    is_release() && php == PHP_LATEST ? ["${TAG_BASE}/frankenscriptling:builder"] : [],
  )

  cache-from = [{
    type = "registry"
    ref  = "${cache_base()}/frankenscriptling:buildcache-php${php}"
  }]
}
