package personal

// ID is this composite screen's identity. Must exist in config.Modules() —
// the list that feeds each resource's permission in the "me" op
// (ProfileDTO.Allows) — without an entry there, platformd silently drops
// this nav item, even though Browser never fails. See config/config.go.
const ID = "personal"

// Label is the nav item's display text.
const Label = "Personal"
