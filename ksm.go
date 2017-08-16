package ksm

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/easonlin404/ksm/aes"
	"github.com/easonlin404/ksm/d"
	"github.com/easonlin404/ksm/rsa"
)

type SPCContainer struct {
	Version           uint32
	Reserved          []byte
	AesKeyIV          []byte //16
	EncryptedAesKey   []byte //128
	CertificateHash   []byte //20
	SPCPlayload       []byte //TODO: struct
	SPCPlayloadLength uint32

	TTLVS map[uint64]TLLVBlock
}

// This function will compute the content key context returned to client by the SKDServer library.
//       incoming server playback context (SPC message)
func GenCKC(playback []byte) error {
	pem := []byte{} //TODO: server pk
	spcv1, err := ParseSPCV1(playback, pem)
	if err != nil {
		return err
	}

	ttlvs := spcv1.TTLVS
	skr1, err := ParseSKR1(ttlvs[Tag_SessionKey_R1])
	if err != nil {
		return err
	}

	appleD := d.AppleD{}
	ask := []byte{} //TODO:

	r2Block := ttlvs[Tag_R2]
	dask, err := appleD.Compute(r2Block.Value, ask)
	if err != nil {
		return err
	}
	fmt.Println(dask)

	b, err := decryptSKR1(*skr1, dask)
	if err != nil {
		return err
	}

	fmt.Println(b)
	fmt.Println(len(b))

	return nil
}

func ParseSPCV1(playback []byte, pem []byte) (*SPCContainer, error) {
	spcContainer := parseSPCContainer(playback)

	spck, err := decryptSPCK(pem, spcContainer.EncryptedAesKey)
	if err != nil {
		return nil, err
	}

	spcPayload, err := decryptSPCpayload(spcContainer, spck)
	if err != nil {
		return nil, err
	}
	fmt.Println(spcPayload)

	printDebugSPC(spcContainer)

	spcContainer.TTLVS = parseTLLVs(spcPayload)

	return spcContainer, nil
}

func parseSPCContainer(playback []byte) *SPCContainer {
	spcContainer := &SPCContainer{}
	spcContainer.Version = binary.BigEndian.Uint32(playback[0:4])
	spcContainer.AesKeyIV = playback[8:24]
	spcContainer.EncryptedAesKey = playback[24:152]
	spcContainer.CertificateHash = playback[152:172]
	spcContainer.SPCPlayloadLength = binary.BigEndian.Uint32(playback[172:176])
	spcContainer.SPCPlayload = playback[176 : 176+spcContainer.SPCPlayloadLength]

	return spcContainer
}

func parseTLLVs(spcpayload []byte) map[uint64]TLLVBlock {
	var m map[uint64]TLLVBlock
	m = make(map[uint64]TLLVBlock)

	fmt.Printf("spcpayload length:%v\n", len(spcpayload))

	for currentOffset := 0; currentOffset < len(spcpayload); {

		tag := binary.BigEndian.Uint64(spcpayload[currentOffset : currentOffset+Field_Tag_Length])
		currentOffset += Field_Tag_Length

		blockLength := binary.BigEndian.Uint32(spcpayload[currentOffset : currentOffset+Field_Block_Length])
		currentOffset += Field_Block_Length

		valueLength := binary.BigEndian.Uint32(spcpayload[currentOffset : currentOffset+Field_Value_Length])
		currentOffset += Field_Value_Length

		//paddingSize := blockLength - valueLength

		value := spcpayload[currentOffset : currentOffset+int(valueLength)]

		var skip bool
		switch tag {
		case Tag_SessionKey_R1:
			fmt.Println("found Tag_SessionKey_R1")
			fmt.Printf("%x\n", tag)
		case Tag_SessionKey_R1_integrity:
			fmt.Println("found Tag_SessionKey_R1_integrity")
			fmt.Printf("%x\n", tag)
		case Tag_AntiReplaySeed:
			fmt.Println("found Tag_AntiReplaySeed")
			fmt.Printf("%x\n", tag)
		case Tag_R2:
			fmt.Println("found Tag_R2")
			fmt.Printf("%x\n", tag)
		case Tag_ReturnRequest:
			fmt.Println("found Tag_ReturnRequest")
			fmt.Printf("%x\n", tag)
		case Tag_AssetID:
			fmt.Println("found Tag_AssetID")
			fmt.Printf("%x\n", tag)
		case Tag_TransactionID:
			fmt.Println("found Tag_TransactionID")
			fmt.Printf("%x\n", tag)
		case Tag_ProtocolVersionsSupported:
			fmt.Println("found Tag_ProtocolVersionsSupported")
			fmt.Printf("%x\n", tag)
		case Tag_ProtocolVersionUsed:
			fmt.Println("found Tag_ProtocolVersionUsed")
			fmt.Printf("%x\n", tag)
		case Tag_treamingIndicator:
			fmt.Println("found Tag_treamingIndicator")
			fmt.Printf("%x\n", tag)
		case Tag_kSKDServerClientReferenceTime:
			fmt.Println("found Tag_kSKDServerClientReferenceTime")
			fmt.Printf("%x\n", tag)
		default:
			skip = true
			//fmt.Println("Undefined TLLVs")
		}

		if skip == false {
			fmt.Printf("blockLength:0x%x\n", blockLength)
			fmt.Printf("valueLength:0x%x\n", valueLength)
			//fmt.Printf("paddingSize:0x%x\n", paddingSize)
			fmt.Printf("Tag value:%s\n\n", hex.EncodeToString(value))

			tllvBlock := TLLVBlock{
				Tag:         tag,
				BlockLength: blockLength,
				ValueLength: valueLength,
				Value:       value,
			}

			m[tag] = tllvBlock

		}

		//TODO: paring ttlv
		currentOffset = currentOffset + int(blockLength)
	}

	return m
}

func ParseSKR1(tllv TLLVBlock) (*SKR1TLLVBlock, error) {
	iv := tllv.Value[16:32]
	payload := tllv.Value[32:128]

	if len(iv) != 16 {
		return nil, errors.New("Wrong SKR1 IV size. Must be 16 bytes expected.")
	}
	if len(payload) != 96 {
		return nil, errors.New("Wrong SKR1 payload size. Must be 96 bytes expected.")
	}

	return &SKR1TLLVBlock{
		TLLVBlock: tllv,
		IV:        iv,
		Payload:   payload,
	}, nil
}

func decryptSKR1(skr1 SKR1TLLVBlock, dask []byte) ([]byte, error) {
	if skr1.Tag != Tag_SessionKey_R1 {
		return nil, errors.New("decryptSKR1 doesn't match Tag_SessionKey_R1 tag")
	}
	return aes.Decrypt(dask, skr1.IV, skr1.Payload)
}

func printDebugSPC(spcContainer *SPCContainer) {
	fmt.Println("========================= Begin SPC Data ===============================")
	fmt.Println("SPC container size -")
	fmt.Println(spcContainer.SPCPlayloadLength)

	fmt.Println("SPC Encryption Key -")
	fmt.Println(hex.EncodeToString(spcContainer.EncryptedAesKey))
	fmt.Println("SPC Encryption IV -")
	fmt.Println(hex.EncodeToString(spcContainer.AesKeyIV))
	fmt.Println("================ SPC TLLV List ================")
	//TODO:
	fmt.Println("[SK ... R1] Integrity Tag --")
	fmt.Println("=========================== End SPC Data =================================")

}

// SPCK = RSA_OAEP d([SPCK])Prv where
// [SPCK] represents the value of SPC message bytes 24-151. Prv represents the server's private key.
func decryptSPCK(pkPem, enSpck []byte) ([]byte, error) {
	if len(enSpck) != 128 {
		return nil, errors.New("Wrong [SPCK] length, must be 128")
	}
	return rsa.OAEPPDecrypt(pkPem, enSpck)
}

// SPC payload = AES_CBCIV d([SPC data])SPCK where
// [SPC data] represents the remaining SPC message bytes beginning at byte 176 (175 + the value of
// SPC message bytes 172-175).
// IV represents the value of SPC message bytes 8-23.
func decryptSPCpayload(spcContainer *SPCContainer, spck []byte) ([]byte, error) {
	spcPayload, err := aes.Decrypt(spck, spcContainer.AesKeyIV, spcContainer.SPCPlayload)
	return spcPayload, err
}
