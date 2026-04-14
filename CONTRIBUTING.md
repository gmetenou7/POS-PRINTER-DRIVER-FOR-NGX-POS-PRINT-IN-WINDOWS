# Contributing to POS Printer Driver Installer

Thank you for your interest in contributing! This project helps Windows users install WinUSB drivers for POS thermal printers, enabling WebUSB access.

## How to Contribute

### Reporting Bugs

- Open an [issue](https://github.com/gmetenou7/POS-PRINTER-DRIVER-FOR-NGX-POS-PRINT-IN-WINDOWS/issues)
- Include your Windows version, printer model (VID:PID), and the error message
- Attach screenshots if possible

### Suggesting Features

- Open an issue with the **enhancement** label
- Describe the use case and expected behavior

### Submitting Code

1. **Fork** the repository
2. **Create a branch** for your feature or fix:
   ```bash
   git checkout -b feature/my-feature
   ```
3. **Make your changes**
4. **Test** on a real POS printer if possible
5. **Commit** with a clear message:
   ```bash
   git commit -m "Add support for XYZ printer"
   ```
6. **Push** and open a **Pull Request**

## Development

### Files Overview

| File | Description |
|------|-------------|
| `install-driver.ps1` | Main PowerShell script (source code) |
| `POS-Printer-Driver-Installer.exe` | Compiled installer (auto-generated) |
| `Install-POS-Printer-Driver.bat` | Batch launcher for the .ps1 script |

### Testing

- Test with at least one POS printer connected via USB
- Verify the driver installs correctly and WebUSB can access the device
- Test on both Windows 10 and Windows 11 if possible

## Code Style

- PowerShell scripts: use clear variable names and comment non-obvious logic
- Keep the installer lightweight and dependency-free

## Adding Printer Support

If you've tested with a printer not listed in the README:

1. Add it to the **Imprimantes testees** table in `README.md`
2. Include the VID:PID if relevant

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
