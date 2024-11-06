# Release notes

This release contains:

* Added byte size format options for file sized based rotation
* Fixed a race condition betweem creating the logfile and rotation
* Add more comprehensive debug logging
* Fixed integer overflow in file size based rotation
* Use mtime as basis for file age computation in max file age rule

For a list of supported features see [here](https://github.com/mojumi-alt/rotee/blob/master/README.md)
