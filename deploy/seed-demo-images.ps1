# Upload commerce demo PNGs to Garage for MEDIA_PUBLIC_BASE image URLs.
param(
  [string]$Endpoint = $env:MEDIA_S3_ENDPOINT,
  [string]$Bucket = $env:MEDIA_S3_BUCKET,
  [string]$AccessKey = $env:MEDIA_S3_ACCESS_KEY,
  [string]$SecretKey = $env:MEDIA_S3_SECRET_KEY,
  [string]$Region = $env:MEDIA_S3_REGION
)
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$assets = Join-Path $root "demo-assets"
if (-not $Endpoint) { $Endpoint = "http://localhost:3900" }
if (-not $Bucket) { $Bucket = "media" }
if (-not $Region) { $Region = "garage" }
if (-not $AccessKey) { $AccessKey = "GKdevmedia00000001" }
if (-not $SecretKey) { $SecretKey = "dev-media-secret-key-for-local-compose-demo" }

$env:AWS_ACCESS_KEY_ID = $AccessKey
$env:AWS_SECRET_ACCESS_KEY = $SecretKey
$env:AWS_DEFAULT_REGION = $Region

foreach ($name in @("Mug", "Tee", "Sticker")) {
  $src = Join-Path $assets "$name.png"
  if (-not (Test-Path $src)) { throw "missing $src" }
  docker run --rm `
    -v "${assets}:/assets:ro" `
    -e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY -e AWS_DEFAULT_REGION `
    amazon/aws-cli:2.15.57 `
    --endpoint-url $Endpoint s3 cp "/assets/$name.png" "s3://$Bucket/demo/$name.png" --content-type image/png
  Write-Host "uploaded demo/$name.png"
}
Write-Host "Set MEDIA_PUBLIC_BASE to garage web base (e.g. http://garage.localhost/$Bucket)"
