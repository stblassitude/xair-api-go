package xair

import "testing"

// TestAddressFormats locks in the X32-vs-X-Air address differences
// documented in WIRE_PROTOCOL.md §8 (padding widths and, for fx/aux
// returns and the main bus, entirely different address roots).
func TestAddressFormats(t *testing.T) {
	c := &Client{}

	xair := &Mixer{Client: c}
	buildXAir(xair, kindTable[KindXR18])

	xairChecks := []struct{ got, want string }{
		{xair.Strip[0].Address(), "/ch/01"},
		{xair.Strip[8].Address(), "/ch/09"},
		{xair.Bus[0].Address(), "/bus/1"},
		{xair.DCA[0].Address(), "/dca/1"},
		{xair.FX[0].Address(), "/fx/1"},
		{xair.FXSend[0].Address(), "/fxsend/1"},
		{xair.FXReturn[0].Address(), "/rtn/1"},
		{xair.AuxReturn[0].Address(), "/rtn/aux"},
		{xair.HeadAmp[0].Address(), "/headamp/01"},
		{xair.LR.Address(), "/lr"},
		{xair.Config.Address(), "/config"},
	}
	for _, c := range xairChecks {
		if c.got != c.want {
			t.Errorf("xair address = %q, want %q", c.got, c.want)
		}
	}
	if len(xair.AuxReturn) != 1 {
		t.Errorf("xair aux return count = %d, want 1", len(xair.AuxReturn))
	}

	x32 := &Mixer{Client: c}
	buildX32(x32, kindTable[KindX32])

	x32Checks := []struct{ got, want string }{
		{x32.Strip[0].Address(), "/ch/01"},
		{x32.Bus[0].Address(), "/bus/01"},
		{x32.DCA[0].Address(), "/dca/1"},
		{x32.FX[0].Address(), "/fx/1"},
		{x32.FXReturn[0].Address(), "/fxrtn/01"},
		{x32.AuxReturn[0].Address(), "/auxin/01"},
		{x32.HeadAmp[0].Address(), "/headamp/001"},
		{x32.HeadAmp[126].Address(), "/headamp/127"},
		{x32.MainStereo.Address(), "/main/st"},
		{x32.MainMono.Address(), "/main/m"},
		{x32.Matrix[0].Address(), "/mtx/01"},
		{x32.Config.Address(), "/config"},
	}
	for _, c := range x32Checks {
		if c.got != c.want {
			t.Errorf("x32 address = %q, want %q", c.got, c.want)
		}
	}
	if len(x32.AuxReturn) != 8 {
		t.Errorf("x32 aux return count = %d, want 8", len(x32.AuxReturn))
	}

	xairK, x32K := kindTable[KindXR18], kindTable[KindX32]
	if got, want := len(xair.FXReturn[0].Send), xairK.numBus+xairK.numFX; got != want {
		t.Errorf("xair send count = %d, want %d (numBus+numFX)", got, want)
	}
	if got, want := len(x32.FXReturn[0].Send), x32K.numBus+x32K.numFX; got != want {
		t.Errorf("x32 send count = %d, want %d (numBus+numFX)", got, want)
	}
}

func TestGEQBandsRejectsUnknownBand(t *testing.T) {
	// A pure validation check: an unknown band must be rejected before
	// any network access is attempted, so this needs no live Client.
	geq := newGEQ(node{c: &Client{}, addr: "/bus/01"})
	if _, err := geq.Slider("31"); err == nil {
		t.Fatal("expected an error for an unknown GEQ band")
	}
	if _, err := enumIndex(geqBands, "31.5"); err != nil {
		t.Fatalf("31.5 should be a valid band: %v", err)
	}
}
