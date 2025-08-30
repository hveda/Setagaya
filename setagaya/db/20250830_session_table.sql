use setagaya;

-- Create user_session table for MySQL session store
-- Schema matches what gorilla/sessions MySQLStore expects
CREATE TABLE IF NOT EXISTS user_session (
    id INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    session_data LONGBLOB,
    created_on TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    modified_on TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    expires_on TIMESTAMP NOT NULL,
    INDEX idx_expires (expires_on)
) CHARSET=utf8mb4;
