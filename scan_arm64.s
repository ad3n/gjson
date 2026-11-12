//go:build !purego

#include "textflag.h"

TEXT ·indexStringSpecialBlock(SB), NOSPLIT, $0-24
	MOVD s_base+0(FP), R0
	MOVD s_len+8(FP), R1
	MOVD R0, R2
	MOVD $34, R3
	VMOV R3, V0.B16
	MOVD $92, R3
	VMOV R3, V1.B16
loop:
	CMP $16, R1
	BLT tail
	VLD1 (R2), [V2.B16]
	VCMEQ V0.B16, V2.B16, V3.B16
	VCMEQ V1.B16, V2.B16, V4.B16
	VORR V3.B16, V4.B16, V3.B16
	VMOV V3.D[0], R3
	VMOV V3.D[1], R4
	CBNZ R3, low
	CBNZ R4, high
	ADD $16, R2
	SUB $16, R1
	B loop
high:
	ADD $8, R2
	MOVD R4, R3
low:
	RBIT R3, R3
	CLZ R3, R3
	ADD R3>>3, R2, R2
	B done
tail:
	CBZ R1, done
	MOVBU (R2), R3
	CMP $34, R3
	BEQ done
	CMP $92, R3
	BEQ done
	ADD $1, R2
	SUB $1, R1
	B tail
done:
	SUB R0, R2, R0
	MOVD R0, ret+16(FP)
	RET
