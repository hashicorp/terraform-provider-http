schema = 1
artifacts {
  zip = [
    "terraform-provider-http_${version}_darwin_amd64.zip",
    "terraform-provider-http_${version}_darwin_arm64.zip",
    "terraform-provider-http_${version}_darwin_arm.zip", # TODO: we had this previously, but it doesn't make sense, does it?
    "terraform-provider-http_${version}_darwin_386.zip", # TODO: ^
    "terraform-provider-http_${version}_freebsd_386.zip",
    "terraform-provider-http_${version}_freebsd_amd64.zip",
    "terraform-provider-http_${version}_freebsd_arm.zip",
    "terraform-provider-http_${version}_linux_386.zip",
    "terraform-provider-http_${version}_linux_amd64.zip",
    "terraform-provider-http_${version}_linux_arm.zip",
    "terraform-provider-http_${version}_linux_arm64.zip",
    "terraform-provider-http_${version}_windows_386.zip",
    "terraform-provider-http_${version}_windows_amd64.zip",
  ]
}