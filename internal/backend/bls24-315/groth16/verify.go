// Copyright 2020 ConsenSys Software Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by gnark DO NOT EDIT

package groth16

import (
	"github.com/consensys/gnark-crypto/ecc/bls24-315/fr"

	curve "github.com/consensys/gnark-crypto/ecc/bls24-315"

	bls24_315witness "github.com/consensys/gnark/internal/backend/bls24-315/witness"

	"errors"
	"fmt"
	"io"
)

var (
	errPairingCheckFailed         = errors.New("pairing doesn't match")
	errCorrectSubgroupCheckFailed = errors.New("points in the proof are not in the correct subgroup")
)

// Verify verifies a proof with given VerifyingKey and publicWitness
func Verify(proof *Proof, vk *VerifyingKey, publicWitness bls24_315witness.Witness) error {

	if len(publicWitness) != (len(vk.G1.K) - 1) {
		return fmt.Errorf("invalid witness size, got %d, expected %d (public - ONE_WIRE)", len(publicWitness), len(vk.G1.K)-1)
	}

	// check that the points in the proof are in the correct subgroup
	if !proof.isValid() {
		return errCorrectSubgroupCheckFailed
	}

	var doubleML curve.GT
	chDone := make(chan error, 1)

	// compute (eKrsδ, eArBs)
	go func() {
		var errML error
		doubleML, errML = curve.MillerLoop([]curve.G1Affine{proof.Krs, proof.Ar}, []curve.G2Affine{vk.G2.deltaNeg, proof.Bs})
		chDone <- errML
		close(chDone)
	}()

	// compute e(Σx.[Kvk(t)]1, -[γ]2)
	var kSum curve.G1Jac
	__publicWitness := make([]fr.Element, len(publicWitness))
	for i := 0; i < len(publicWitness); i++ {
		__publicWitness[i] = publicWitness[i].ToRegular()
	}
	kSum.MultiExp(vk.G1.K[1:], __publicWitness)
	kSum.AddMixed(&vk.G1.K[0])
	var kSumAff curve.G1Affine
	kSumAff.FromJacobian(&kSum)

	right, err := curve.MillerLoop([]curve.G1Affine{kSumAff}, []curve.G2Affine{vk.G2.gammaNeg})
	if err != nil {
		return err
	}

	// wait for (eKrsδ, eArBs)
	err = <-chDone
	if err != nil {
		return err
	}

	right = curve.FinalExponentiation(&right, &doubleML)
	if !vk.e.Equal(&right) {
		return errPairingCheckFailed
	}
	return nil
}

// ExportSolidity not implemented for BLS24-315
func (vk *VerifyingKey) ExportSolidity(w io.Writer) error {
	return errors.New("not implemented")
}
