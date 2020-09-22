## 1.4 (September 22, 2020)

NOTES:

* Add support for `http_archive`.


## 1.3.1 (Unreleased)

BUG FIXES:

* Modified some of the documentation to work a bit better in the registry [GH-42]

NOTES:

* The provider now uses the binary driver for acceptance tests [GH-39]

## 1.2.0 (March 17, 2020)

IMPROVEMENTS:

* Switch to v1.7.0 of the standalone plugin SDK ([#35](https://github.com/terraform-providers/terraform-provider-http/issues/35))
* Added response_headers to datasource ([#31](https://github.com/terraform-providers/terraform-provider-http/issues/31))

BUG FIXES:

* Fix request error message to include the `err` and not just url ([#26](https://github.com/terraform-providers/terraform-provider-http/issues/26))

## 1.1.1 (May 01, 2019)

* This release includes an upgrade to the Terraform SDK, in an effort to help align with what other providers are releasing with, as we lead up to Core v0.12. It should have no noticeable impact on the provider.

## 1.1.0 (April 18, 2019)

IMPROVEMENTS:

* The provider is now compatible with Terraform v0.12, while retaining compatibility with prior versions.

## 1.0.1 (January 03, 2018)

* Allow `charset` argument on `Content-Type` ([#5](https://github.com/terraform-providers/terraform-provider-http/issues/5))

## 1.0.0 (September 14, 2017)

* add content type for ADFS FederationMetadata.xml ([#4](https://github.com/terraform-providers/terraform-provider-http/issues/4))

## 0.1.0 (June 20, 2017)

NOTES:

* Same functionality as that of Terraform 0.9.8. Repacked as part of [Provider Splitout](https://www.hashicorp.com/blog/upcoming-provider-changes-in-terraform-0-10/)
