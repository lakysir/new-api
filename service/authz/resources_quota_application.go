package authz

const (
	ResourceQuotaApplication = "quota_application"
	ActionApplyQuota         = "apply"
)

var QuotaApplicationApply = Permission{Resource: ResourceQuotaApplication, Action: ActionApplyQuota}

func init() {
	RegisterResource(ResourceDefinition{
		Resource: ResourceQuotaApplication,
		LabelKey: "Quota Applications",
		Actions: []ActionDefinition{
			{
				Action:         ActionApplyQuota,
				LabelKey:       "Apply for user quota",
				DescriptionKey: "Search users, submit quota applications, and view your own application progress.",
			},
		},
	})
}
