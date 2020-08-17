package ibmcloud

func IsNotFound(err error) bool {
	return err != nil && err.Error() == "not found"
}
