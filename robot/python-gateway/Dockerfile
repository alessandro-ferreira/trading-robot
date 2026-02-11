# Use the official lightweight Python image
FROM python:3.12-slim

# Set the working directory
WORKDIR /app

# Copy requirements first for better cache
COPY requirements.txt requirements.in ./

# Install dependencies
RUN pip install --upgrade pip && \
    pip install -r requirements.txt

# Copy the rest of the application code
COPY . .

# Expose the gRPC port
EXPOSE 50051

# Run the main application
CMD ["python", "main.py", "--config", "../config.toml"]
