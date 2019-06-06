# Configurating Travis

## Settings

Some tests are performed against a real IBM cloud account.
The following environment variable must be set:


* BLUEMIX_API_KEY: the [IBM Cloud API Key](https://cloud.ibm.com/iam/apikeys) of the account use to run tests
* BLUEMIX_ORG: the IBM cloud organization
* BLUEMIX_SPACE: the IBM cloud space
* BLUEMIX_REGION: the IBM cloud region