//go:build tuning_bundle && linux && amd64

package runtime

import _ "embed"

//go:embed assets/linux-amd64.tar.gz
var bundle []byte

//go:embed assets/linux-amd64.json
var metadata []byte
