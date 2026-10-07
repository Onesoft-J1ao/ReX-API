package authz

const (
	ResourceRedemption = "redemption"

	// ActionCreateCode and ActionDeleteCode are separate from a plain write
	// because creating a redemption code mints spendable balance: it is the one
	// administrative action whose output is money, so it is worth granting
	// independently of reading the code list.
	ActionCreateCode = "create_code"
	ActionDeleteCode = "delete_code"
)

var (
	RedemptionRead       = Permission{Resource: ResourceRedemption, Action: ActionRead}
	RedemptionCreateCode = Permission{Resource: ResourceRedemption, Action: ActionCreateCode}
	RedemptionDeleteCode = Permission{Resource: ResourceRedemption, Action: ActionDeleteCode}
)

func init() {
	RegisterResource(ResourceDefinition{
		Resource: ResourceRedemption,
		LabelKey: "Redemption Codes",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "Read redemption codes",
				DescriptionKey: "View the redemption code list and individual codes.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionCreateCode,
				LabelKey:       "Create redemption codes",
				DescriptionKey: "Create or edit redemption codes. Each code grants spendable balance when redeemed.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionDeleteCode,
				LabelKey:       "Delete redemption codes",
				DescriptionKey: "Delete individual codes, batch-delete by id, or purge all invalid codes.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
		},
	})
}
