//go:build tuning_bundle && linux && arm64

package runtime

import _ "embed"

//go:embed assets/linux-arm64.tar.gz
var bundle []byte

//go:embed assets/linux-arm64.json
var metadata []byte
