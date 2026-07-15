// Package domain contains dependency-free vocabulary shared by the design
// pipeline's bounded contexts. Package aliases preserve local APIs while
// making cross-package assignments type-safe.
package domain

type AcceptanceLevel string

const (
	AcceptanceDraft            AcceptanceLevel = "draft"
	AcceptanceStructural       AcceptanceLevel = "structural"
	AcceptanceConnectivity     AcceptanceLevel = "connectivity"
	AcceptanceERCDRC           AcceptanceLevel = "erc-drc"
	AcceptanceERCDRCUnderscore AcceptanceLevel = "erc_drc"
	// AcceptanceERCClean is a legacy schematic-IR wire spelling. It remains
	// distinct from the workflow's erc-drc value for compatibility.
	AcceptanceERCClean                       AcceptanceLevel = "erc_clean"
	AcceptanceReadable                       AcceptanceLevel = "readable"
	AcceptanceFabricationCandidate           AcceptanceLevel = "fabrication-candidate"
	AcceptanceFabricationCandidateUnderscore AcceptanceLevel = "fabrication_candidate"
)

type ComponentRole string

const (
	ComponentRoleConnector           ComponentRole = "connector"
	ComponentRoleInputConnector      ComponentRole = "input_connector"
	ComponentRoleOutputConnector     ComponentRole = "output_connector"
	ComponentRoleResistor            ComponentRole = "resistor"
	ComponentRoleCurrentLimiter      ComponentRole = "current_limiter"
	ComponentRolePullup              ComponentRole = "pullup"
	ComponentRoleCapacitor           ComponentRole = "capacitor"
	ComponentRoleDecouplingCapacitor ComponentRole = "decoupling_capacitor"
	ComponentRoleBulkCapacitor       ComponentRole = "bulk_capacitor"
	ComponentRoleInductor            ComponentRole = "inductor"
	ComponentRoleDiode               ComponentRole = "diode"
	ComponentRoleIndicatorLED        ComponentRole = "indicator_led"
	ComponentRoleIC                  ComponentRole = "ic"
	ComponentRoleSensor              ComponentRole = "sensor"
	ComponentRoleRegulator           ComponentRole = "regulator"
	ComponentRoleTransistor          ComponentRole = "transistor"
	ComponentRoleBJT                 ComponentRole = "bjt"
	ComponentRoleMOSFET              ComponentRole = "mosfet"
	ComponentRoleSwitch              ComponentRole = "switch"
	ComponentRoleCrystal             ComponentRole = "crystal"
	ComponentRoleOscillator          ComponentRole = "oscillator"
	ComponentRoleProtection          ComponentRole = "protection"
	ComponentRoleFuse                ComponentRole = "fuse"
	ComponentRoleTVS                 ComponentRole = "tvs"
	ComponentRolePowerSymbol         ComponentRole = "power_symbol"
	ComponentRoleGroundSymbol        ComponentRole = "ground_symbol"
	ComponentRoleTestpoint           ComponentRole = "testpoint"
	ComponentRoleGeneric             ComponentRole = "generic"
)

type NetRole string

const (
	NetRolePower        NetRole = "power"
	NetRoleGround       NetRole = "ground"
	NetRoleSignal       NetRole = "signal"
	NetRoleClock        NetRole = "clock"
	NetRoleAnalog       NetRole = "analog"
	NetRoleHighCurrent  NetRole = "high_current"
	NetRoleDifferential NetRole = "differential"
	NetRoleUnknown      NetRole = "unknown"
	NetRolePowerPos     NetRole = "power_pos"
	NetRolePowerNeg     NetRole = "power_neg"
	NetRoleReturn       NetRole = "return"
	NetRoleFeedback     NetRole = "feedback"
	NetRoleBias         NetRole = "bias"
	NetRoleShield       NetRole = "shield"
	NetRoleBus          NetRole = "bus"
	NetRoleNoConnect    NetRole = "no_connect"
)
