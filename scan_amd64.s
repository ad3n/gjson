//go:build !purego

#include "textflag.h"

TEXT ·indexStringSpecialBlock(SB), NOSPLIT, $0-24
	MOVQ s_base+0(FP), SI
	MOVQ s_len+8(FP), CX
	XORQ AX, AX
	MOVQ $0x2222222222222222, DX
	MOVQ DX, X0
	PUNPCKLQDQ X0, X0
	MOVQ $0x5c5c5c5c5c5c5c5c, DX
	MOVQ DX, X1
	PUNPCKLQDQ X1, X1
loop:
	CMPQ CX, $16
	JB tail
	MOVOU (SI)(AX*1), X2
	MOVOU X2, X3
	PCMPEQB X0, X2
	PCMPEQB X1, X3
	POR X3, X2
	PMOVMSKB X2, DX
	TESTL DX, DX
	JNZ found
	ADDQ $16, AX
	SUBQ $16, CX
	JMP loop
found:
	BSFL DX, DX
	ADDQ DX, AX
	JMP done
tail:
	TESTQ CX, CX
	JZ done
	MOVBLZX (SI)(AX*1), DX
	CMPB DL, $34
	JE done
	CMPB DL, $92
	JE done
	INCQ AX
	DECQ CX
	JMP tail
done:
	MOVQ AX, ret+16(FP)
	RET
