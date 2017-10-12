The packages inside `vendor/` directoy contain bad go code. This "vendor"
directory exists to avoid its content from being parsed by ListPackages when
running on the whole dep code base.
