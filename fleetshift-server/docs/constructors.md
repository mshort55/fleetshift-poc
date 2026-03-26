Constructors:

- Use the "Option" function pattern, with exported option functions
- Pass required arguments either in a config struct or positional arguments
- Ensure the object is in a valid state after construction. LAZY ASSIGNMENT TO FIELDS SHOULD NOT BE REQUIRED. If there are circular references, this is a smell.