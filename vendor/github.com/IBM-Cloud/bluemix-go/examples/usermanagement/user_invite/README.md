# Invite User

This example invites user to an account.

Example: 

```
go run main.go -userEmail new@in.ibm.com -accountID <account-id> 
```

# Invite user with acces groups

This example invites user to an account with the comma separated list of access groups

Example: 

```
go run main.go -userEmail new@in.ibm.com -accountID <account-id> -accessGroups "<AccessGroupId-****>,<AccessGroupId-***>"
```

# Invite user with IAM policy

This example invites user to an account with an IAM Policy

Example: 

```
go run main.go -userEmail new@in.ibm.com -accountID <account-id> -roles "Opera
tor,Writer" -service "<service>" -resourceGroupID "<resourceGroupID>" 
```

# Invite user with Classic infrastructure Permissions

This example invites user to an account with Comma separated list of classic infrastructure permissions

Example: 

```
go run main.go -userEmail new@in.ibm.com -accountID <account-id> -infraPermission "LOCKBOX_MANAGE,I
P_ADD,FIREWALL_RULE_MANAGE,LOADBALANCER_MANAGE"
```

# Invite user with CloudFoundry roles 

This example invites user to an account with Comma separated list of orgnization and space roles

Example: 

```
go run main.go -userEmail new@in.ibm.com -accountID <account-id> -org <org-name> -space <space-name> -region <region> -orgRoles "BillingManager,Manager" -spaceRoles "Developer,Manager"
```



