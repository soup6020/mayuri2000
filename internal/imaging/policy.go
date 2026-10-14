package imaging

import _ "embed"

// magickPolicy is ImageMagick's security policy: only the formats the
// effects use, no delegates, no "@file" reads. See policy/policy.xml.
//
//go:embed policy/policy.xml
var magickPolicy []byte
