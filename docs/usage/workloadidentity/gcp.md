# Using `WorkloadIdentity` for `Google Cloud DNS`

To use `WorkloadIdentity` for `Google Cloud DNS`, you can create a `WorkloadIdentity` resource in the project namespace in the Garden cluster with the necessary configuration for Google Cloud authentication.

Note that the `spec.targetSystem.type` has to be set to `gcp` although the type of the DNS provider is `google-clouddns`. This allows to use the same `WorkloadIdentity` for different types of GCP resources, e.g., for infrastructure purposes and for `DNSProvider` purposes.

To create a `WorkloadIdentity`, follow the instructions in the [GCP Workload Identity Federation documentation](https://gardener.cloud/docs/extensions/infrastructure-extensions/gardener-extension-provider-gcp/usage/#gcp-workload-identity-federation) of the Google Cloud Provider extension.

For the required permissions, please refer to the [Google Cloud DNS Provider](https://github.com/gardener/external-dns-management/tree/master/docs/google-cloud-dns#required-permissions).
