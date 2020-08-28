package iam

import (
	"fmt"

	"github.com/IBM-Cloud/bluemix-go/api/iam/iamv1"
	"github.com/IBM-Cloud/bluemix-go/crn"
	"github.com/IBM-Cloud/bluemix-go/models"
	"github.com/IBM-Cloud/bluemix-go/session"
	"github.com/IBM-Cloud/bluemix-go/utils"
)

type ServiceRolesGetter func(session *session.Session, serviceName, roleName string) (crn.CRN, error)

var _ ServiceRolesGetter = GetServiceRoleCRN

// GetServiceRoleCRN searches serviceName's service roles for 'roleName' and returns its CRN.
// If serviceName is empty, searches system roles.
// If roleName is empty, searches for a "Manager" role if it exists. If the Manager role isn't present, the first role is chosen.
func GetServiceRoleCRN(session *session.Session, serviceName, roleName string) (crn.CRN, error) {
	iamClient, err := iamv1.New(session)
	if err != nil {
		return crn.CRN{}, err
	}

	serviceRolesAPI := iamClient.ServiceRoles()
	var roles []models.PolicyRole

	if serviceName == "" {
		roles, err = serviceRolesAPI.ListSystemDefinedRoles()
	} else {
		roles, err = serviceRolesAPI.ListServiceRoles(serviceName)
	}
	if err != nil {
		return crn.CRN{}, err
	}

	var roleID crn.CRN

	if roleName != "" {
		roleMatch, err := utils.FindRoleByName(roles, roleName)
		if err != nil {
			return crn.CRN{}, err
		}
		roleID = roleMatch.ID
	} else {
		if len(roles) == 0 {
			return crn.CRN{}, fmt.Errorf("The service has no roles defined for its bindings")
		}
		managerRole, err := getManagerRole(roles)
		if err != nil {
			// No Manager role found
			roleID = roles[0].ID
		} else {
			roleID = managerRole.ID
		}
	}
	return roleID, nil
}

func getManagerRole(roles []models.PolicyRole) (models.PolicyRole, error) {
	for _, role := range roles {
		if role.DisplayName == "Manager" {
			return role, nil
		}
	}
	return models.PolicyRole{}, fmt.Errorf("No Manager role found")
}
