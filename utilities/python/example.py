import time
import logging
from writetracer import WriteTracer

# Configure logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')

def main():
    print("Starting WriteTracer Python Example")
    
    # Example 1: Using Context Manager (Recommended)
    print("\n--- Example 1: Context Manager ---")
    with WriteTracer() as tracer:
        print(f"Tracking active for PID {tracer.pid}")
        for i in range(5):
            print(f"Doing work... iteration {i}")
            # Perform some file I/O to be traced
            with open("python_trace.log", "a") as f:
                f.write(f"Iteration {i}\n")
            time.sleep(1)
    print("Tracking stopped (Context Manager exited)")

    # Example 2: Manual Registration
    print("\n--- Example 2: Manual Registration ---")
    tracer = WriteTracer()
    if tracer.register():
        try:
            print(f"Tracking active for PID {tracer.pid}")
            time.sleep(2)
            # More work...
            with open("python_trace.log", "a") as f:
                f.write("Manual mode work\n")
        finally:
            tracer.unregister()
            print("Tracking stopped (Manual unregister)")

if __name__ == "__main__":
    main()
