#!/usr/bin/env sh
# Upload commerce demo product images to Garage/S3 for MEDIA_PUBLIC_BASE URLs.
# Usage (from pf-commerce/deploy with pf-media garage running):
#   MEDIA_S3_ENDPOINT=http://localhost:3900 MEDIA_S3_BUCKET=portfolio-demo \
#   AWS_ACCESS_KEY_ID=... AWS_SECRET_ACCESS_KEY=... ./seed-demo-images.sh
set -eu
ROOT="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
ASSETS="$ROOT/demo-assets"
ENDPOINT="${MEDIA_S3_ENDPOINT:-http://localhost:3900}"
BUCKET="${MEDIA_S3_BUCKET:-portfolio-demo}"
REGION="${MEDIA_S3_REGION:-garage}"

if [ ! -d "$ASSETS" ]; then
  echo "missing $ASSETS" >&2
  exit 1
fi

for name in Mug Tee Sticker; do
  src="$ASSETS/$name.png"
  if [ ! -f "$src" ]; then
    echo "missing $src" >&2
    exit 1
  fi
  aws --endpoint-url "$ENDPOINT" --region "$REGION" s3 cp "$src" "s3://$BUCKET/demo/$name.png" --content-type image/png
  echo "uploaded demo/$name.png"
done

echo "Set MEDIA_PUBLIC_BASE to your garage web base (e.g. http://garage.localhost/$BUCKET)"
