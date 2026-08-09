# =============================================================================
# Variables (overridable via environment variables; see Makefile for defaults)
# =============================================================================

variable "TAG_BASE" {
  default = "paularlott"
}

variable "CACHE_TAG_BASE" {
  default = ""
}

variable "FRANKENPHP_VERSION" {
  default = "1.12.7"
}

variable "SCRIPTLING_VERSION" {
  default = "v0.20.1"
}

variable "PHP_VERSIONS" {
  type    = list(string)
  default = ["8.4.24", "8.5.9"]
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
  result = substr(version, 1, -1)
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
  targets = ["frankenscriptling"]
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

  labels = {
    "org.opencontainers.image.title"       = "Frankenscriptling"
    "org.opencontainers.image.description" = "FrankenPHP with the Scriptling PHP extension"
    "org.opencontainers.image.version"     = "${no_prefix(SCRIPTLING_VERSION)}-php${php}"
  }

  args = {
    FRANKENPHP_VERSION = "${FRANKENPHP_VERSION}"
    PHP_VERSION        = "${php}"
    SCRIPTLING_VERSION = "${SCRIPTLING_VERSION}"
  }

  tags = [
    "${TAG_BASE}/frankenscriptling:${no_prefix(SCRIPTLING_VERSION)}-php${php}",
    "${TAG_BASE}/frankenscriptling:${no_prefix(SCRIPTLING_VERSION)}-php${major_minor(php)}",
  ]

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
