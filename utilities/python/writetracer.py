import os
import requests
import logging

class WriteTracer:
    """
    Client for the write-tracer REST API.
    Allows registering and unregistering PIDs for monitoring.
    This class can be used as a context manager.
    """
    def __init__(self, url="http://localhost:9092"):
        """
        Initialize the client.
        
        Args:
            url (str): The base URL of the write-tracer REST API.
        """
        self.base_url = url.rstrip('/')
        self.pid = None
        self.logger = logging.getLogger(__name__)

    def register(self, pid=None):
        """
        Register a PID for tracking.
        
        Args:
            pid (int, optional): The PID to register. Defaults to the current process ID.
            
        Returns:
            bool: True if registration was successful, False otherwise.
        """
        if pid is None:
            pid = os.getpid()
        
        self.pid = pid
        try:
            response = requests.post(
                f"{self.base_url}/pids",
                json={"pid": pid},
                headers={"Content-Type": "application/json"},
                timeout=2
            )
            response.raise_for_status()
            self.logger.info(f"Successfully registered PID {pid}")
            return True
        except requests.exceptions.RequestException as e:
            self.logger.error(f"Failed to register PID {pid}: {e}")
            return False

    def unregister(self, pid=None):
        """
        Unregister a PID from tracking.
        
        Args:
            pid (int, optional): The PID to unregister. Defaults to the registered PID.
            
        Returns:
            bool: True if unregistration was successful, False otherwise.
        """
        if pid is None:
            pid = self.pid
            
        if pid is None:
            self.logger.warning("No PID specified to unregister")
            return False

        try:
            response = requests.delete(
                f"{self.base_url}/pids/{pid}",
                timeout=2
            )
            response.raise_for_status()
            self.logger.info(f"Successfully unregistered PID {pid}")
            return True
        except requests.exceptions.RequestException as e:
            self.logger.error(f"Failed to unregister PID {pid}: {e}")
            return False

    def __enter__(self):
        """
        Enter the runtime context related to this object.
        Registers the current PID.
        """
        self.register()
        return self

    def __exit__(self, exc_type, exc_value, traceback):
        """
        Exit the runtime context related to this object.
        Unregisters the current PID.
        """
        self.unregister()
