/*
Copyright © 2020 ConsenSys

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package groth16

import (
	"fmt"
	"testing"

	"github.com/consensys/gnark/backend"
	backend_bls377 "github.com/consensys/gnark/backend/bls377"
	groth16_bls377 "github.com/consensys/gnark/backend/bls377/groth16"
	backend_bw761 "github.com/consensys/gnark/backend/bw761"
	mimcbls377 "github.com/consensys/gnark/crypto/hash/mimc/bls377"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/gadgets/algebra/fields"
	"github.com/consensys/gnark/gadgets/algebra/sw"
	"github.com/consensys/gnark/gadgets/hash/mimc"
	"github.com/consensys/gurvy"
	"github.com/consensys/gurvy/bls377"
	fr_bls377 "github.com/consensys/gurvy/bls377/fr"
)

//--------------------------------------------------------------------
// utils

const preimage string = "7808462342289447506325013279997289618334122576263655295146895675168642919487"

func generateBls377InnerProof(t *testing.T, vk *groth16_bls377.VerifyingKey, proof *groth16_bls377.Proof) {

	// create a mock circuit: knowing the preimage of a hash using mimc
	circuit := frontend.New()
	hFunc, err := mimc.NewMiMCGadget("seed", gurvy.BLS377)
	if err != nil {
		t.Fatal(err)
	}
	res := hFunc.Hash(&circuit, circuit.SECRET_INPUT("private_data"))
	circuit.MUSTBE_EQ(res, circuit.PUBLIC_INPUT("public_hash"))

	// build the r1cs from the circuit
	r1cs := backend_bls377.New(&circuit)

	// compute the public/private inputs using a real mimc
	var preimage, publicHash fr_bls377.Element
	b := mimcbls377.Sum("seed", preimage.Bytes())
	publicHash.SetBytes(b)

	// create the correct assignment
	correctAssignment := backend.NewAssignment()
	correctAssignment.Assign(backend.Secret, "private_data", preimage)
	correctAssignment.Assign(backend.Public, "public_hash", publicHash)

	// generate the data to return for the bls377 proof
	var pk groth16_bls377.ProvingKey
	groth16_bls377.Setup(&r1cs, &pk, vk)
	proof, err = groth16_bls377.Prove(&r1cs, &pk, correctAssignment)
	if err != nil {
		t.Fatal(err)
	}

	// before returning verifies that the proof passes on bls377
	proofOk, err := groth16_bls377.Verify(proof, vk, correctAssignment)
	if err != nil {
		t.Fatal(err)
	}
	if !proofOk {
		t.Fatal("error during bls377 proof verification")
	}
}

func newPointAffineCircuitG2(circuit *frontend.CS, s string) *sw.G2Aff {
	x := fields.NewFp2Elmt(circuit, circuit.SECRET_INPUT(s+"x0"), circuit.SECRET_INPUT(s+"x1"))
	y := fields.NewFp2Elmt(circuit, circuit.SECRET_INPUT(s+"y0"), circuit.SECRET_INPUT(s+"y1"))
	return sw.NewPointG2Aff(circuit, x, y)
}

func newPointCircuitG1(circuit *frontend.CS, s string) *sw.G1Aff {
	return sw.NewPointG1Aff(circuit,
		circuit.SECRET_INPUT(s+"0"),
		circuit.SECRET_INPUT(s+"1"),
	)
}

func allocateInnerVk(circuit *frontend.CS, vk *groth16_bls377.VerifyingKey, innerVk *VerifyingKey) {

	innerVk.E.C0.B0.X = circuit.ALLOCATE(vk.E.C0.B0.A0)
	innerVk.E.C0.B0.Y = circuit.ALLOCATE(vk.E.C0.B0.A1)
	innerVk.E.C0.B1.X = circuit.ALLOCATE(vk.E.C0.B1.A0)
	innerVk.E.C0.B1.Y = circuit.ALLOCATE(vk.E.C0.B1.A1)
	innerVk.E.C0.B2.X = circuit.ALLOCATE(vk.E.C0.B2.A0)
	innerVk.E.C0.B2.Y = circuit.ALLOCATE(vk.E.C0.B2.A1)
	innerVk.E.C1.B0.X = circuit.ALLOCATE(vk.E.C1.B0.A0)
	innerVk.E.C1.B0.Y = circuit.ALLOCATE(vk.E.C1.B0.A1)
	innerVk.E.C1.B1.X = circuit.ALLOCATE(vk.E.C1.B1.A0)
	innerVk.E.C1.B1.Y = circuit.ALLOCATE(vk.E.C1.B1.A1)
	innerVk.E.C1.B2.X = circuit.ALLOCATE(vk.E.C1.B2.A0)
	innerVk.E.C1.B2.Y = circuit.ALLOCATE(vk.E.C1.B2.A1)

	allocateG2(circuit, &innerVk.G2.DeltaNeg, &vk.G2.DeltaNeg)
	allocateG2(circuit, &innerVk.G2.GammaNeg, &vk.G2.GammaNeg)

	innerVk.G1 = make([]sw.G1Aff, len(vk.G1.K))
	for i := 0; i < len(vk.G1.K); i++ {
		allocateG1(circuit, &innerVk.G1[i], &vk.G1.K[i])
	}
}

func allocateInnerProof(circuit *frontend.CS, innerProof *Proof) {
	var Ar, Krs *sw.G1Aff
	var Bs *sw.G2Aff
	Ar = newPointCircuitG1(circuit, "Ar")
	Krs = newPointCircuitG1(circuit, "Krs")
	Bs = newPointAffineCircuitG2(circuit, "Bs")
	innerProof.Ar = *Ar
	innerProof.Krs = *Krs
	innerProof.Bs = *Bs
}

func allocateG2(circuit *frontend.CS, g2 *sw.G2Aff, g2Circuit *bls377.G2Affine) {
	g2.X.X = circuit.ALLOCATE(g2Circuit.X.A0)
	g2.X.Y = circuit.ALLOCATE(g2Circuit.X.A1)
	g2.Y.X = circuit.ALLOCATE(g2Circuit.Y.A0)
	g2.Y.Y = circuit.ALLOCATE(g2Circuit.Y.A1)
}

func allocateG1(circuit *frontend.CS, g1 *sw.G1Aff, g1Circuit *bls377.G1Affine) {
	g1.X = circuit.ALLOCATE(g1Circuit.X)
	g1.Y = circuit.ALLOCATE(g1Circuit.Y)
}

//--------------------------------------------------------------------
// test

func TestVerifier(t *testing.T) {

	t.Skip("wip")

	// get the data
	var vk groth16_bls377.VerifyingKey
	var proof groth16_bls377.Proof
	generateBls377InnerProof(t, &vk, &proof)

	// create an empty circuit
	circuit := frontend.New()

	// pairing data
	var pairingInfo sw.PairingContext
	pairingInfo.Extension = fields.GetBLS377ExtensionFp12(&circuit)
	pairingInfo.AteLoop = 9586122913090633729

	// allocate the verifying key
	var innerVk VerifyingKey
	allocateInnerVk(&circuit, &vk, &innerVk)

	// create secret inputs corresponding to the proof
	var innerProof Proof
	allocateInnerProof(&circuit, &innerProof)

	// get the name of the public inputs of the inner snark (that will become the public inputs of the outer snark)
	publicInputNames := []string{"public_hash"}

	// create the verifier circuit
	Verify(&circuit, pairingInfo, innerVk, innerProof, publicInputNames)

	// create r1cs
	r1cs := backend_bw761.New(&circuit)

	fmt.Println(r1cs.NbConstraints)

}
