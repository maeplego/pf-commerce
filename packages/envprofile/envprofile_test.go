package envprofile

import "testing"

func TestNormalize(t *testing.T) {
	if Normalize("") != Development {
		t.Fatal("empty -> development")
	}
	if Normalize("STAGE") != Staging {
		t.Fatal("STAGE -> staging")
	}
	if Normalize("prod") != Production {
		t.Fatal("prod -> production")
	}
}

func TestValidateCommercial(t *testing.T) {
	if err := ValidateCommercial(Staging, true, "http://idp", "COMMERCE_ENV", "COMMERCE_DEV_AUTH"); err == nil {
		t.Fatal("staging+devAuth must fail")
	}
	if err := ValidateCommercial(Staging, false, "", "COMMERCE_ENV", "COMMERCE_DEV_AUTH"); err == nil {
		t.Fatal("staging without OIDC must fail")
	}
	if err := ValidateCommercial(Staging, false, "http://idp", "COMMERCE_ENV", "COMMERCE_DEV_AUTH"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCommercial(Development, true, "", "COMMERCE_ENV", "COMMERCE_DEV_AUTH"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCommercial("weird", false, "http://idp", "COMMERCE_ENV", "COMMERCE_DEV_AUTH"); err == nil {
		t.Fatal("unsupported env must fail")
	}
}
