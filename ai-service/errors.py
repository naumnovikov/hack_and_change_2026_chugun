class AIError(Exception):
    """Public error with a safe, non-provider-specific explanation."""

    def __init__(self, code: str, message: str, status: int = 503):
        super().__init__(message)
        self.code, self.message, self.status = code, message, status
