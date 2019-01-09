#include "stdafx.h"



PSID GetSidForUser(LPCWSTR host, LPCWSTR user) {
	SID *newsid = NULL;
	DWORD cbSid = 0;
	LPWSTR refDomain = NULL;
	DWORD cchRefDomain = 0;
	SID_NAME_USE use;
	std::string narrowdomain;
	BOOL bRet = LookupAccountName(host, user, newsid, &cbSid, refDomain, &cchRefDomain, &use);
	if (bRet) {
		// this should *never* happen, because we didn't pass in a buffer large enough for
		// the sid or the domain name.
		return NULL;
	}
	DWORD err = GetLastError();
	if (ERROR_INSUFFICIENT_BUFFER != err) {
		// we don't know what happened
		return NULL;
	}
	newsid = (SID *) new BYTE[cbSid];
	ZeroMemory(newsid, cbSid);

	refDomain = new wchar_t[cchRefDomain + 1];
	ZeroMemory(refDomain, (cchRefDomain + 1) * sizeof(wchar_t));

	// try it again
	bRet = LookupAccountName(host, user, newsid, &cbSid, refDomain, &cchRefDomain, &use);
	if (!bRet) {
		WcaLog(LOGMSG_STANDARD, "Failed to lookup account name %d", GetLastError());
		goto cleanAndFail;
	}
	if (!IsValidSid(newsid)) {
		WcaLog(LOGMSG_STANDARD, "New SID is invalid");
		goto cleanAndFail;
	}
	
	toMbcs(narrowdomain, refDomain);
	WcaLog(LOGMSG_STANDARD, "Got SID from %s", narrowdomain.c_str());
	delete[] refDomain;
	return newsid;

cleanAndFail:
	if (newsid) {
		delete[](BYTE*)newsid;
	}
	if (refDomain) {
		delete[] refDomain;
	}
	return NULL;
}
bool RemovePrivileges(PSID AccountSID, LSA_HANDLE PolicyHandle, LPCWSTR rightToAdd)
{
	LSA_UNICODE_STRING lucPrivilege;
	NTSTATUS ntsResult;

	// Create an LSA_UNICODE_STRING for the privilege names.
	if (!InitLsaString(&lucPrivilege, rightToAdd))
	{
		WcaLog(LOGMSG_STANDARD, "Failed InitLsaString");
		return false;
	}

	ntsResult = LsaRemoveAccountRights(
		PolicyHandle,  // An open policy handle.
		AccountSID,    // The target SID.
		FALSE,
		&lucPrivilege, // The privileges.
		1              // Number of privileges.
	);
	if (ntsResult == 0)
	{
		WcaLog(LOGMSG_STANDARD, "Privilege removed");
		return true;
	}
	else
	{
		WcaLog(LOGMSG_STANDARD, "Privilege was not removed - %lu \n",
			LsaNtStatusToWinError(ntsResult));
	}
	return false;
}

bool AddPrivileges(PSID AccountSID, LSA_HANDLE PolicyHandle, LPCWSTR rightToAdd)
{
	LSA_UNICODE_STRING lucPrivilege;
	NTSTATUS ntsResult;

	// Create an LSA_UNICODE_STRING for the privilege names.
	if (!InitLsaString(&lucPrivilege, rightToAdd))
	{
		WcaLog(LOGMSG_STANDARD, "Failed InitLsaString");
		return false;
	}

	ntsResult = LsaAddAccountRights(
		PolicyHandle,  // An open policy handle.
		AccountSID,    // The target SID.
		&lucPrivilege, // The privileges.
		1              // Number of privileges.
	);
	if (ntsResult == 0)
	{
		WcaLog(LOGMSG_STANDARD, "Privilege added");
		return true;
	}
	else
	{
		WcaLog(LOGMSG_STANDARD, "Privilege was not added - %lu \n",
			LsaNtStatusToWinError(ntsResult));
	}

	return false;
}


// returned value must be freed with LsaClose()

LSA_HANDLE GetPolicyHandle()
{
	LSA_OBJECT_ATTRIBUTES ObjectAttributes;
	NTSTATUS ntsResult;
	LSA_HANDLE lsahPolicyHandle;

	// Object attributes are reserved, so initialize to zeros.
	ZeroMemory(&ObjectAttributes, sizeof(ObjectAttributes));

	//Initialize an LSA_UNICODE_STRING to the server name.

	// Get a handle to the Policy object.
	ntsResult = LsaOpenPolicy(
		NULL, // always assume local system
		&ObjectAttributes, //Object attributes.
		POLICY_ALL_ACCESS, //Desired access permissions.
		&lsahPolicyHandle  //Receives the policy handle.
	);

	if (ntsResult != 0)
	{
		// An error occurred. Display it as a win32 error code.
		WcaLog(LOGMSG_STANDARD, "OpenPolicy returned %lu\n",
			LsaNtStatusToWinError(ntsResult));
		return NULL;
	}
	return lsahPolicyHandle;
}


bool InitLsaString(
	PLSA_UNICODE_STRING pLsaString,
	LPCWSTR pwszString
)
{
	DWORD dwLen = 0;

	if (NULL == pLsaString)
		return FALSE;

	if (NULL != pwszString)
	{
		dwLen = wcslen(pwszString);
		if (dwLen > 0x7ffe)   // String is too large
			return FALSE;
	}

	// Store the string.
	pLsaString->Buffer = (WCHAR *)pwszString;
	pLsaString->Length = (USHORT)dwLen * sizeof(WCHAR);
	pLsaString->MaximumLength = (USHORT)(dwLen + 1) * sizeof(WCHAR);

	return TRUE;
}
void BuildExplicitAccessWithSid(EXPLICIT_ACCESS_W &data, PSID pSID, DWORD perms, ACCESS_MODE mode, DWORD inheritance) 
{
	data.grfAccessPermissions = perms;
	data.grfAccessMode = mode;
	data.grfInheritance = inheritance;
	data.Trustee.pMultipleTrustee = NULL;
	data.Trustee.MultipleTrusteeOperation = NO_MULTIPLE_TRUSTEE;
	data.Trustee.TrusteeForm = TRUSTEE_IS_SID;
	data.Trustee.TrusteeType = TRUSTEE_IS_USER;
	data.Trustee.ptstrName = (LPTSTR)pSID;
}

int EnableServiceForUser(const std::wstring& service, LPCWSTR domain, LPCWSTR user)
{
    int ret = 0;
    SC_HANDLE hscm = OpenSCManager(NULL, NULL, SC_MANAGER_ALL_ACCESS | GENERIC_ALL | READ_CONTROL);
	if (!hscm) {
        ret = GetLastError();
		WcaLog(LOGMSG_STANDARD, "failed to open scm %d\n", ret);
		return ret;
	}
	
	DWORD dwInfo = 0;
	BYTE bigbuf[8192];
	DWORD pcbBytes = 8192;
	char *ssec = NULL;
	PSID sid = NULL;
	PSECURITY_DESCRIPTOR psec = (PSECURITY_DESCRIPTOR)bigbuf;
	EXPLICIT_ACCESSW ea;
	memset(&ea, 0, sizeof(EXPLICIT_ACCESSW));
	PACL pacl = NULL, pNewAcl = NULL;
	BOOL bDaclDefaulted = FALSE;
	BOOL bDaclPresent = FALSE;
	DWORD dwError = 0;
	SECURITY_DESCRIPTOR sd;
    std::string asciiservice;
    toMbcs(asciiservice, service.c_str());
    WcaLog(LOGMSG_STANDARD, "attempting to open %s", asciiservice.c_str());
    SC_HANDLE hService = OpenServiceW(hscm, (LPCWSTR)service.c_str(), SERVICE_ALL_ACCESS | READ_CONTROL | WRITE_DAC);
	if (!hService) {
		WcaLog(LOGMSG_STANDARD,"Failed to open service %d\n", GetLastError());
		goto cleanAndReturn;
	}
	
	
	if (!QueryServiceObjectSecurity(hService, DACL_SECURITY_INFORMATION , psec, 8192, &pcbBytes)) {
		WcaLog(LOGMSG_STANDARD,"Failed to query security info %d\n", GetLastError());
		goto cleanAndReturn;
	}
	if ((sid = GetSidForUser(domain, user)) == NULL) {
		WcaLog(LOGMSG_STANDARD,"Failed to get sid\n");
		goto cleanAndReturn;
	}
	// Get the DACL...

	if (!GetSecurityDescriptorDacl(psec, &bDaclPresent, &pacl, &bDaclDefaulted))
	{
		WcaLog(LOGMSG_STANDARD,"Failed to get security dacl %d \n", GetLastError());
		goto cleanAndReturn;
	}

	// Build the ACE.

	BuildExplicitAccessWithSid(ea, sid, SERVICE_START | SERVICE_STOP | READ_CONTROL | DELETE, SET_ACCESS, NO_INHERITANCE);

	dwError = SetEntriesInAcl(1, &ea, pacl, &pNewAcl);

	if (dwError != ERROR_SUCCESS)
	{
		WcaLog(LOGMSG_STANDARD,"Failed to set security dacl %d \n", dwError);
		goto cleanAndReturn;
	}


	// Initialize a NEW Security Descriptor.

	if (!InitializeSecurityDescriptor(&sd, SECURITY_DESCRIPTOR_REVISION))
	{
		WcaLog(LOGMSG_STANDARD,"Failed to initialize security descriptor %d \n", GetLastError());
		goto cleanAndReturn;
	}


	// Set the new DACL in the Security Descriptor.

	if (!SetSecurityDescriptorDacl(&sd, TRUE, pNewAcl, FALSE))
	{
		WcaLog(LOGMSG_STANDARD,"Failed to set security descriptor Dacl %d \n", GetLastError());
		goto cleanAndReturn;
	}



	// Set the new DACL for the service object.

	if (!SetServiceObjectSecurity(hService, DACL_SECURITY_INFORMATION, &sd))
	{
		WcaLog(LOGMSG_STANDARD,"Failed to set security object %d \n", GetLastError());
		goto cleanAndReturn;
	}

	
cleanAndReturn:
	if (hscm) {
		CloseServiceHandle(hscm);
	}
	if (hService) {
		CloseServiceHandle(hscm);
	}
	if (sid) {
		delete[](BYTE *) sid;
	}
    return ret;
}