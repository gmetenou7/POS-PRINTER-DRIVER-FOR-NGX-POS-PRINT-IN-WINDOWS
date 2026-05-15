package escpos

// QR code and 1-D barcode helpers. All commands target the Epson ESC/POS
// dialect (GS ( k for QR, GS k for 1-D), which is implemented by virtually
// every modern thermal printer including the Chinese clones.

type QRECC int

const (
	QRECCLow      QRECC = 48 // ~7% recovery
	QRECCMedium   QRECC = 49 // ~15%
	QRECCQuartile QRECC = 50 // ~25%
	QRECCHigh     QRECC = 51 // ~30%
)

// QRCode draws a QR code containing `data`. `module` is the dot size
// (1..16, recommended 4..8). `ecc` controls error correction.
//
// Wraps the standard sequence:
//   1. set model (GS ( k pL pH cn fn n1 n2)
//   2. set size  (GS ( k ...)
//   3. set EC    (GS ( k ...)
//   4. store     (GS ( k pL pH cn fn data)
//   5. print     (GS ( k ...)
func (b *Builder) QRCode(data string, module int, ecc QRECC) *Builder {
	if module < 1 {
		module = 4
	}
	if module > 16 {
		module = 16
	}
	if ecc == 0 {
		ecc = QRECCMedium
	}

	// 1) Model 2
	b.buf.Write([]byte{GS, '(', 'k', 0x04, 0x00, '1', 'A', 0x32, 0x00})
	// 2) Size
	b.buf.Write([]byte{GS, '(', 'k', 0x03, 0x00, '1', 'C', byte(module)})
	// 3) Error correction
	b.buf.Write([]byte{GS, '(', 'k', 0x03, 0x00, '1', 'E', byte(ecc)})

	// 4) Store data — pL pH = data length + 3 (cn + fn + m)
	raw := []byte(data)
	n := len(raw) + 3
	pL := byte(n & 0xFF)
	pH := byte((n >> 8) & 0xFF)
	header := []byte{GS, '(', 'k', pL, pH, '1', 'P', '0'}
	b.buf.Write(header)
	b.buf.Write(raw)

	// 5) Print
	b.buf.Write([]byte{GS, '(', 'k', 0x03, 0x00, '1', 'Q', '0'})
	return b
}

type BarcodeType byte

const (
	BarcodeUPCA    BarcodeType = 65
	BarcodeUPCE    BarcodeType = 66
	BarcodeEAN13   BarcodeType = 67
	BarcodeEAN8    BarcodeType = 68
	BarcodeCODE39  BarcodeType = 69
	BarcodeITF     BarcodeType = 70
	BarcodeCODABAR BarcodeType = 71
	BarcodeCODE93  BarcodeType = 72
	BarcodeCODE128 BarcodeType = 73
)

type BarcodeHRI int

const (
	BarcodeHRINone   BarcodeHRI = 0
	BarcodeHRIAbove  BarcodeHRI = 1
	BarcodeHRIBelow  BarcodeHRI = 2
	BarcodeHRIBoth   BarcodeHRI = 3
)

// Barcode prints a 1-D barcode. `height` is in dots (default 100, max 255),
// `widthMul` controls the module width (2..6, default 3). `hri` controls
// whether the human-readable interpretation is printed.
//
// For CODE128, prepend `{A`, `{B`, or `{C` to data for the desired subset.
func (b *Builder) Barcode(t BarcodeType, data string, height int, widthMul int, hri BarcodeHRI) *Builder {
	if height < 1 {
		height = 100
	}
	if height > 255 {
		height = 255
	}
	if widthMul < 2 {
		widthMul = 3
	}
	if widthMul > 6 {
		widthMul = 6
	}
	// HRI position
	b.buf.Write([]byte{GS, 'H', byte(hri)})
	// Module width
	b.buf.Write([]byte{GS, 'w', byte(widthMul)})
	// Height
	b.buf.Write([]byte{GS, 'h', byte(height)})
	// Print barcode using "function B" (GS k m pL data) which accepts
	// arbitrary-length data with explicit length byte.
	raw := []byte(data)
	b.buf.Write([]byte{GS, 'k', byte(t), byte(len(raw))})
	b.buf.Write(raw)
	return b
}
