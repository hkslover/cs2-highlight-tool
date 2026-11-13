package producegame

import _ "embed"

// BrandVPK is the embedded watermark VPK mounted when branding is enabled,
// independently of POV HUD. An existing user-supplied brand.vpk is preserved,
// matching the POV lifecycle.
//
//go:embed assets/brand.vpk
var BrandVPK []byte
